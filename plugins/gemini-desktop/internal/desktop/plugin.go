package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ProxyURL     string `yaml:"proxy_url"`
	CallbackPort int    `yaml:"callback_port"`
}

type Plugin struct {
	config           Config
	mu               sync.Mutex
	pending          map[string]Login
	root             context.Context
	cancel           context.CancelFunc
	generate         func(context.Context, string, string, string, func(string) error) error
	generateSelected func(context.Context, string, string, string, NativeModel, func(string) error) error
	discover         func(context.Context, string, string) ([]NativeModel, error)
	hostProxy        string
}

func Build(configYAML []byte) (pluginapi.Plugin, error) {
	var config Config
	if len(configYAML) > 0 {
		if err := yaml.Unmarshal(configYAML, &config); err != nil {
			return pluginapi.Plugin{}, failure(400, "Invalid plugin configuration")
		}
	}
	if config.CallbackPort == 0 {
		config.CallbackPort = 51123
	}
	if config.CallbackPort < 1024 || config.CallbackPort > 65535 {
		return pluginapi.Plugin{}, failure(400, "callback_port must be between 1024 and 65535")
	}
	root, cancel := context.WithCancel(context.Background())
	p := &Plugin{config: config, pending: make(map[string]Login), root: root, cancel: cancel, generate: Generate, generateSelected: GenerateSelected, discover: DiscoverModels}
	return pluginapi.Plugin{
		Metadata: pluginapi.Metadata{Name: "Gemini Desktop Preview", Version: "0.2.0-preview", Author: "Local integration", GitHubRepository: "https://github.com/twuijri/cpa-plugin-api-key-manager", ConfigFields: []pluginapi.ConfigField{
			{Name: "callback_port", Type: pluginapi.ConfigFieldTypeInteger, Default: 51123, Description: "Loopback OAuth callback port. Remote browser logins require forwarding or submitting the callback URL in the OAuth panel."},
			{Name: "proxy_url", Type: pluginapi.ConfigFieldTypeString, Default: "", Description: "Upstream HTTP CONNECT or SOCKS proxy for native gRPC; inherits proxy-url when empty."},
		}},
		Capabilities: pluginapi.Capabilities{AuthProvider: p, ModelProvider: p, Executor: p, ExecutorModelScope: pluginapi.ExecutorModelScopeOAuth, ExecutorInputFormats: []string{"openai"}, ExecutorOutputFormats: []string{"openai"}},
	}, nil
}

func (p *Plugin) Close()             { p.cancel(); p.mu.Lock(); clear(p.pending); p.mu.Unlock() }
func (p *Plugin) Identifier() string { return Provider }

func (p *Plugin) observeHost(host pluginapi.HostConfigSummary) {
	p.mu.Lock()
	p.hostProxy = host.ProxyURL
	p.mu.Unlock()
}

func (p *Plugin) ParseAuth(_ context.Context, req pluginapi.AuthParseRequest) (pluginapi.AuthParseResponse, error) {
	p.observeHost(req.Host)
	var kind struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(req.RawJSON, &kind) != nil || kind.Type != Provider {
		return pluginapi.AuthParseResponse{Handled: false}, nil
	}
	c, err := ParseCredential(req.RawJSON)
	if err != nil {
		return pluginapi.AuthParseResponse{}, err
	}
	auth, err := Auth(c)
	if req.FileName != "" {
		auth.ID, auth.FileName = req.FileName, req.FileName
	}
	return pluginapi.AuthParseResponse{Handled: true, Auth: auth}, err
}

func (p *Plugin) StartLogin(_ context.Context, req pluginapi.AuthLoginStartRequest) (pluginapi.AuthLoginStartResponse, error) {
	p.observeHost(req.Host)
	redirect := fmt.Sprintf("http://localhost:%d/auth", p.config.CallbackPort)
	login, err := Start(redirect)
	if err != nil {
		return pluginapi.AuthLoginStartResponse{}, err
	}
	p.mu.Lock()
	for state, session := range p.pending {
		if time.Now().After(session.Expires) {
			delete(p.pending, state)
		}
	}
	if len(p.pending) >= 32 {
		p.mu.Unlock()
		return pluginapi.AuthLoginStartResponse{}, failure(429, "Too many pending Gemini logins")
	}
	p.pending[login.State] = login
	p.mu.Unlock()
	return pluginapi.AuthLoginStartResponse{Provider: Provider, URL: login.URL, State: login.State, ExpiresAt: login.Expires, Metadata: map[string]any{"redirect_uri": redirect}}, nil
}

var statePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func (p *Plugin) PollLogin(ctx context.Context, req pluginapi.AuthLoginPollRequest) (pluginapi.AuthLoginPollResponse, error) {
	if !statePattern.MatchString(req.State) {
		return pluginapi.AuthLoginPollResponse{}, failure(400, "Invalid OAuth state")
	}
	p.mu.Lock()
	login, found := p.pending[req.State]
	if found && time.Now().After(login.Expires) {
		delete(p.pending, req.State)
		found = false
	}
	p.mu.Unlock()
	if !found {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "Gemini login expired or was already consumed"}, nil
	}
	if strings.TrimSpace(req.Host.AuthDir) == "" {
		return pluginapi.AuthLoginPollResponse{}, failure(500, "Host credential directory is unavailable")
	}
	path := filepath.Join(req.Host.AuthDir, ".oauth-"+Provider+"-"+req.State+".oauth")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending}, nil
	}
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, failure(500, "Could not read OAuth callback")
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<10 {
		return pluginapi.AuthLoginPollResponse{}, failure(400, "Invalid OAuth callback file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, failure(500, "Could not read OAuth callback")
	}
	var callback struct {
		Code  string `json:"code"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &callback) != nil || callback.State != req.State {
		return pluginapi.AuthLoginPollResponse{}, failure(400, "OAuth callback state did not match")
	}
	p.mu.Lock()
	_, found = p.pending[req.State]
	delete(p.pending, req.State)
	p.mu.Unlock()
	if !found {
		return pluginapi.AuthLoginPollResponse{}, failure(409, "OAuth callback already consumed")
	}
	if err = os.Remove(path); err != nil {
		return pluginapi.AuthLoginPollResponse{}, failure(500, "Could not consume OAuth callback")
	}
	if callback.Error != "" {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "Google authorization was declined"}, nil
	}
	c, err := Exchange(ctx, req.HTTPClient, login, callback.Code, callback.State)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	p.discoverCredentialModels(ctx, &c)
	auth, err := Auth(c)
	return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusSuccess, Auth: auth}, err
}

func (p *Plugin) RefreshAuth(ctx context.Context, req pluginapi.AuthRefreshRequest) (pluginapi.AuthRefreshResponse, error) {
	p.observeHost(req.Host)
	c, err := ParseCredential(req.StorageJSON)
	if err != nil {
		return pluginapi.AuthRefreshResponse{}, err
	}
	c, err = Refresh(ctx, req.HTTPClient, c)
	if err != nil {
		return pluginapi.AuthRefreshResponse{}, err
	}
	p.discoverCredentialModels(ctx, &c)
	auth, err := Auth(c)
	if req.AuthID != "" {
		auth.ID, auth.FileName = req.AuthID, req.AuthID
	}
	return pluginapi.AuthRefreshResponse{Auth: auth, NextRefreshAfter: auth.NextRefreshAfter}, err
}

func models() pluginapi.ModelResponse {
	return pluginapi.ModelResponse{Provider: Provider, Models: []pluginapi.ModelInfo{{ID: Model, Object: "model", OwnedBy: "google", Type: Provider, Name: Model, DisplayName: "Gemini Desktop — default model", Description: "Native Gemini Desktop text conversations. The account's default model is selected by Google; this alias does not pin a model version.", SupportedInputModalities: []string{"text"}, SupportedOutputModalities: []string{"text"}}}}
}
func (p *Plugin) StaticModels(_ context.Context, req pluginapi.StaticModelRequest) (pluginapi.ModelResponse, error) {
	p.observeHost(req.Host)
	return models(), nil
}
func (p *Plugin) ModelsForAuth(_ context.Context, req pluginapi.AuthModelRequest) (pluginapi.ModelResponse, error) {
	p.observeHost(req.Host)
	c, err := ParseCredential(req.StorageJSON)
	if err != nil {
		return pluginapi.ModelResponse{}, err
	}
	response := pluginapi.ModelResponse{Provider: Provider}
	seen := map[string]bool{Model: true}
	for _, selected := range c.Models {
		alias := selected.Alias()
		if !validNativeSelection(selected) || alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		response.Models = append(response.Models, pluginapi.ModelInfo{ID: alias, Object: "model", OwnedBy: "google", Type: Provider, Name: alias, DisplayName: selected.Title, Description: "Native Gemini Desktop model selected from this account's available modes.", SupportedInputModalities: []string{"text"}, SupportedOutputModalities: []string{"text"}})
	}
	response.Models = append(response.Models, models().Models...)
	return response, nil
}

func (p *Plugin) proxyFor(c Credential) string {
	if c.ProxyURL != "" {
		return c.ProxyURL
	}
	if p.config.ProxyURL != "" {
		return p.config.ProxyURL
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hostProxy
}

func (p *Plugin) discoverCredentialModels(ctx context.Context, c *Credential) {
	// Discovery must not prevent a successfully rotated token from being saved.
	if p.discover == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if discovered, err := p.discover(ctx, c.AccessToken, p.proxyFor(*c)); err == nil {
		c.Models = discovered
	}
}

func selectedModel(c Credential, alias string) (NativeModel, error) {
	if alias == Model {
		return NativeModel{}, nil
	}
	for _, model := range c.Models {
		if validNativeSelection(model) && model.Alias() == alias {
			return model, nil
		}
	}
	if len(c.Models) == 0 {
		return NativeModel{}, failure(409, "Gemini account model list is unavailable; refresh this credential to discover its models")
	}
	return NativeModel{}, failure(404, "Requested Gemini Desktop model is not available for this account")
}

func validNativeSelection(model NativeModel) bool {
	return validateNativeModel(model) == nil
}

func (p *Plugin) generateFor(ctx context.Context, req pluginapi.ExecutorRequest, c Credential, proxyURL, prompt string, snapshot func(string) error) error {
	if req.Model == Model {
		return p.generate(ctx, c.AccessToken, proxyURL, prompt, snapshot)
	}
	selected, err := selectedModel(c, req.Model)
	if err != nil {
		return err
	}
	return p.generateSelected(ctx, c.AccessToken, proxyURL, prompt, selected, snapshot)
}

func (p *Plugin) prepare(ctx context.Context, req pluginapi.ExecutorRequest) (Credential, string, string, error) {
	if req.Format != "openai" {
		return Credential{}, "", "", failure(400, "Gemini Desktop requires the OpenAI Chat Completions format")
	}
	prompt, err := conversation(req.Payload)
	if err != nil {
		return Credential{}, "", "", err
	}
	c, err := ParseCredential(req.StorageJSON)
	if err != nil {
		return Credential{}, "", "", err
	}
	if _, err := selectedModel(c, req.Model); err != nil {
		return Credential{}, "", "", err
	}
	if c.AccessToken == "" || !time.Now().Before(c.ExpiresAt) {
		return Credential{}, "", "", failure(401, "Gemini credential needs refresh; automatic host refresh or the credential panel will renew and persist it")
	}
	return c, prompt, p.proxyFor(c), nil
}

func (p *Plugin) requestContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(p.root, cancel)
	return ctx, func() { stop(); cancel() }
}

func (p *Plugin) Execute(ctx context.Context, req pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	ctx, cancel := p.requestContext(ctx)
	defer cancel()
	c, prompt, proxyURL, err := p.prepare(ctx, req)
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	var text string
	err = p.generateFor(ctx, req, c, proxyURL, prompt, func(snapshot string) error { text = snapshot; return nil })
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	id := fmt.Sprintf("chatcmpl-geminidesktop-%d", time.Now().UnixNano())
	return pluginapi.ExecutorResponse{Headers: headers(), Payload: completion(id, req.Model, text, time.Now().Unix())}, nil
}

func (p *Plugin) ExecuteStream(ctx context.Context, req pluginapi.ExecutorRequest) (pluginapi.ExecutorStreamResponse, error) {
	ctx, cancel := p.requestContext(ctx)
	c, prompt, proxyURL, err := p.prepare(ctx, req)
	if err != nil {
		cancel()
		return pluginapi.ExecutorStreamResponse{}, err
	}
	chunks := make(chan pluginapi.ExecutorStreamChunk, 8)
	id, created := fmt.Sprintf("chatcmpl-geminidesktop-%d", time.Now().UnixNano()), time.Now().Unix()
	go func() {
		defer cancel()
		defer close(chunks)
		send := func(payload []byte, err error) bool {
			select {
			case chunks <- pluginapi.ExecutorStreamChunk{Payload: payload, Err: err}:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !send(streamChunk(id, req.Model, created, map[string]any{"role": "assistant", "content": ""}, nil), nil) {
			return
		}
		previous := ""
		err := p.generateFor(ctx, req, c, proxyURL, prompt, func(text string) error {
			if !strings.HasPrefix(text, previous) {
				return failure(502, "Gemini revised streamed text; retry without streaming")
			}
			delta := strings.TrimPrefix(text, previous)
			previous = text
			if delta != "" && !send(streamChunk(id, req.Model, created, map[string]any{"content": delta}, nil), nil) {
				return ctx.Err()
			}
			return nil
		})
		if err != nil {
			send(nil, err)
			return
		}
		send(streamChunk(id, req.Model, created, map[string]any{}, "stop"), nil)
	}()
	return pluginapi.ExecutorStreamResponse{Headers: map[string][]string{"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"}}, Chunks: chunks}, nil
}

func (p *Plugin) CountTokens(context.Context, pluginapi.ExecutorRequest) (pluginapi.ExecutorResponse, error) {
	return pluginapi.ExecutorResponse{}, failure(501, "Native Gemini Desktop does not expose exact token counts in this preview")
}

func (p *Plugin) HttpRequest(context.Context, pluginapi.ExecutorHTTPRequest) (pluginapi.ExecutorHTTPResponse, error) {
	return pluginapi.ExecutorHTTPResponse{}, failure(501, "Arbitrary upstream HTTP requests are not supported by Gemini Desktop")
}
