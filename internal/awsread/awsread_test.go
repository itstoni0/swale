package awsread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/logging"
	"github.com/aws/smithy-go/middleware"
)

const (
	testAccessKey = "AKIATESTSWALEKEY000001"
	testSecret    = "swale-test-secret-value"
	testToken     = "swale-test-session-token"
	fileAccessKey = "AKIATESTFROMFILE000000"
	testAccount   = "111122223333"
	testRegion    = "us-east-1"
)

// denyClient fails the test when Do runs.
// A write must not reach this client.
type denyClient struct {
	calls atomic.Int32
}

func (c *denyClient) Do(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("network call")
}

// scriptClient records each request and returns a scripted response.
type scriptClient struct {
	calls atomic.Int32
	url   atomic.Value
	fn    func(*http.Request) (*http.Response, error)
}

func (c *scriptClient) Do(req *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	if req != nil && req.URL != nil {
		c.url.Store(req.URL.String())
	}
	return c.fn(req)
}

func (c *scriptClient) lastURL() string {
	v, _ := c.url.Load().(string)
	return v
}

// captureLogger writes each SDK log line into a buffer.
type captureLogger struct {
	buf *bytes.Buffer
}

func (c captureLogger) Logf(_ logging.Classification, format string, v ...interface{}) {
	fmt.Fprintf(c.buf, format, v...)
	c.buf.WriteByte('\n')
}

func setTestAWSEnv(t *testing.T, endpoint string) {
	t.Helper()
	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	configFile := filepath.Join(dir, "config")
	credBody := "[default]\naws_access_key_id = " + fileAccessKey + "\naws_secret_access_key = file-secret-not-used\naws_session_token = file-token-not-used\n"
	if err := os.WriteFile(credFile, []byte(credBody), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte("[default]\nregion = ap-southeast-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_ACCESS_KEY_ID", testAccessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", testSecret)
	t.Setenv("AWS_SESSION_TOKEN", testToken)
	t.Setenv("AWS_REGION", testRegion)
	t.Setenv("AWS_DEFAULT_REGION", testRegion)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_SDK_LOAD_CONFIG", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)
	t.Setenv("AWS_CONFIG_FILE", configFile)
	t.Setenv("AWS_ENDPOINT_URL", endpoint)
	t.Setenv("AWS_CA_BUNDLE", "")
}

func xmlResponse(body string) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        http.StatusText(http.StatusOK),
		Header:        http.Header{"Content-Type": []string{"application/xml"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func callerIdentityXML(account string) string {
	return "<GetCallerIdentityResponse><GetCallerIdentityResult><Account>" + account + "</Account><Arn>arn:aws:iam::" + account + ":user/discovery</Arn><UserId>AIDATEST</UserId></GetCallerIdentityResult></GetCallerIdentityResponse>"
}

func openForTest(t *testing.T, settings Settings) *Session {
	t.Helper()
	sess, err := Open(context.Background(), settings)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	return sess
}

// TestAssumeRoleMakesNoNetworkCall proves a write fails before HTTP.
func TestAssumeRoleMakesNoNetworkCall(t *testing.T) {
	setTestAWSEnv(t, "")
	var logBuf bytes.Buffer
	client := &denyClient{}
	sess := openForTest(t, Settings{
		HTTPClient: client,
		Logger:     testLogger(&logBuf),
	})

	stsClient := sts.NewFromConfig(sess.Config())
	_, err := stsClient.AssumeRole(context.Background(), &sts.AssumeRoleInput{
		RoleArn:         aws.String("arn:aws:iam::111122223333:role/discovery"),
		RoleSessionName: aws.String("swale-test"),
	})
	if !errors.Is(err, ErrNotReadOnly) {
		t.Fatalf("AssumeRole error %v does not wrap %v", err, ErrNotReadOnly)
	}
	if client.calls.Load() != 0 {
		t.Fatalf("AssumeRole made %d network call(s)", client.calls.Load())
	}
	if !strings.Contains(logBuf.String(), "AssumeRole") {
		t.Fatalf("log %q does not name AssumeRole", logBuf.String())
	}
	assertNoSecrets(t, logBuf.String())
}

// TestCreateVpcMakesNoCall proves the allowlist refuses CreateVpc before the handler.
func TestCreateVpcMakesNoCall(t *testing.T) {
	var logBuf bytes.Buffer
	called := false
	err := runOperation(t, "CreateVpc", &logBuf, func(context.Context, interface{}) (interface{}, middleware.Metadata, error) {
		called = true
		return nil, middleware.Metadata{}, errors.New("handler called")
	})
	if called {
		t.Fatal("CreateVpc reached the handler")
	}
	if !errors.Is(err, ErrNotReadOnly) {
		t.Fatalf("CreateVpc error %v does not wrap %v", err, ErrNotReadOnly)
	}
	if !strings.Contains(logBuf.String(), "CreateVpc") {
		t.Fatalf("log %q does not name CreateVpc", logBuf.String())
	}
}

func TestAllowlist(t *testing.T) {
	allowed := []string{"DescribeVpcs", "ListBuckets", "GetCallerIdentity", "Describe", "List", "Get"}
	for _, name := range allowed {
		t.Run(name, func(t *testing.T) {
			called := false
			err := runOperation(t, name, nil, func(context.Context, interface{}) (interface{}, middleware.Metadata, error) {
				called = true
				return "ok", middleware.Metadata{}, nil
			})
			if err != nil {
				t.Fatalf("operation %s: unexpected error: %v", name, err)
			}
			if !called {
				t.Fatalf("operation %s did not reach the handler", name)
			}
		})
	}

	refused := []string{
		"",
		"CreateVpc",
		"AssumeRole",
		"AssumeRoleWithWebIdentity",
		"DeleteBucket",
		"PutObject",
		"getCallerIdentity",
		"GetSessionToken",
		"GetFederationToken",
		"GetDelegatedAccessToken",
		"GetWebIdentityToken",
	}
	for _, name := range refused {
		t.Run(name, func(t *testing.T) {
			called := false
			err := runOperation(t, name, nil, func(context.Context, interface{}) (interface{}, middleware.Metadata, error) {
				called = true
				return nil, middleware.Metadata{}, nil
			})
			if called {
				t.Fatalf("operation %q reached the handler", name)
			}
			if !errors.Is(err, ErrNotReadOnly) {
				t.Fatalf("operation %q error %v does not wrap %v", name, err, ErrNotReadOnly)
			}
		})
	}
}

func TestGetSessionTokenMakesNoNetworkCall(t *testing.T) {
	setTestAWSEnv(t, "")
	client := &denyClient{}
	sess := openForTest(t, Settings{HTTPClient: client})
	stsClient := sts.NewFromConfig(sess.Config())
	_, err := stsClient.GetSessionToken(context.Background(), &sts.GetSessionTokenInput{})
	if !errors.Is(err, ErrNotReadOnly) {
		t.Fatalf("GetSessionToken error %v does not wrap %v", err, ErrNotReadOnly)
	}
	if client.calls.Load() != 0 {
		t.Fatalf("GetSessionToken made %d network call(s)", client.calls.Load())
	}
}

func TestCallerIdentity(t *testing.T) {
	setTestAWSEnv(t, "https://127.0.0.1:9297")
	var ours bytes.Buffer
	var sdkLog bytes.Buffer
	httpClient := &scriptClient{fn: func(*http.Request) (*http.Response, error) {
		return xmlResponse(callerIdentityXML(testAccount)), nil
	}}
	sess := openForTest(t, Settings{
		Load: func(ctx context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
			cfg, err := config.LoadDefaultConfig(ctx, optFns...)
			if err != nil {
				return aws.Config{}, err
			}
			cfg.Logger = captureLogger{buf: &sdkLog}
			cfg.ClientLogMode = aws.LogSigning | aws.LogRequest | aws.LogRequestWithBody | aws.LogResponse | aws.LogResponseWithBody
			return cfg, nil
		},
		HTTPClient: httpClient,
		Logger:     testLogger(&ours),
	})

	got, err := sess.Caller(context.Background())
	if err != nil {
		t.Fatalf("Caller: unexpected error: %v", err)
	}
	if got.AccountID != testAccount {
		t.Fatalf("account ID %q, want %q", got.AccountID, testAccount)
	}
	if got.Region != testRegion {
		t.Fatalf("region %q, want %q", got.Region, testRegion)
	}
	if httpClient.calls.Load() == 0 {
		t.Fatal("GetCallerIdentity made no HTTP call")
	}
	if !strings.Contains(httpClient.lastURL(), "https://127.0.0.1:9297") {
		t.Fatalf("request URL %q does not use AWS_ENDPOINT_URL", httpClient.lastURL())
	}
	if !strings.Contains(ours.String(), testAccount) || !strings.Contains(ours.String(), testRegion) {
		t.Fatalf("log %q does not record the account and the region", ours.String())
	}
	assertNoSecrets(t, ours.String())
	assertNoSecrets(t, sdkLog.String())
	if strings.Contains(sdkLog.String(), "AWS4-HMAC-SHA256") || strings.Contains(sdkLog.String(), "Credential=") {
		t.Fatalf("SDK log contains a signature: %s", sdkLog.String())
	}
}

func TestCallerWithNilLogger(t *testing.T) {
	setTestAWSEnv(t, "")
	httpClient := &scriptClient{fn: func(*http.Request) (*http.Response, error) {
		return xmlResponse(callerIdentityXML(testAccount)), nil
	}}
	sess := openForTest(t, Settings{HTTPClient: httpClient})
	got, err := sess.Caller(context.Background())
	if err != nil {
		t.Fatalf("Caller: unexpected error: %v", err)
	}
	if got.AccountID != testAccount || got.Region != testRegion {
		t.Fatalf("Caller = %#v", got)
	}
}

func TestCallerIdentityHTTPError(t *testing.T) {
	setTestAWSEnv(t, "")
	var logBuf bytes.Buffer
	httpClient := &scriptClient{fn: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	}}
	sess := openForTest(t, Settings{
		HTTPClient: httpClient,
		Logger:     testLogger(&logBuf),
	})
	_, err := sess.Caller(context.Background())
	if err == nil {
		t.Fatal("Caller: got no error, want an error")
	}
	if errors.Is(err, ErrNotReadOnly) {
		t.Fatalf("Caller: read error %v", err)
	}
	if !strings.Contains(logBuf.String(), "get caller identity failed") {
		t.Fatalf("log %q does not record the failure", logBuf.String())
	}
	assertNoSecrets(t, logBuf.String())
	if strings.Contains(logBuf.String(), "transport failed") {
		t.Fatalf("log %q includes the SDK error", logBuf.String())
	}
}

func TestCallerIdentityEmptyAccount(t *testing.T) {
	setTestAWSEnv(t, "")
	var logBuf bytes.Buffer
	body := "<GetCallerIdentityResponse><GetCallerIdentityResult><Account></Account><Arn>arn:aws:iam::111122223333:user/discovery</Arn><UserId>AIDATEST</UserId></GetCallerIdentityResult></GetCallerIdentityResponse>"
	httpClient := &scriptClient{fn: func(*http.Request) (*http.Response, error) {
		return xmlResponse(body), nil
	}}
	sess := openForTest(t, Settings{
		HTTPClient: httpClient,
		Logger:     testLogger(&logBuf),
	})
	_, err := sess.Caller(context.Background())
	if err == nil {
		t.Fatal("Caller: got no error, want an empty account error")
	}
	if !strings.Contains(err.Error(), "account ID is empty") {
		t.Fatalf("Caller error %q does not name an empty account ID", err)
	}
	assertNoSecrets(t, logBuf.String())
}

func TestOpenRequiresRegion(t *testing.T) {
	_, err := Open(context.Background(), Settings{
		Load: func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, nil
		},
	})
	if err == nil {
		t.Fatal("Open: got no error, want an empty region error")
	}
	if !strings.Contains(err.Error(), "region is empty") {
		t.Fatalf("Open error %q does not name an empty region", err)
	}
}

func TestOpenReturnsLoadError(t *testing.T) {
	loadErr := errors.New("load failed")
	_, err := Open(context.Background(), Settings{
		Load: func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, loadErr
		},
	})
	if err == nil {
		t.Fatal("Open: got no error, want a load error")
	}
	if !errors.Is(err, loadErr) {
		t.Fatalf("Open error %v does not wrap %v", err, loadErr)
	}
	if !strings.HasPrefix(err.Error(), "load aws config:") {
		t.Fatalf("Open error %q does not start with %q", err, "load aws config:")
	}
}

func TestOpenUsesEnvCredentials(t *testing.T) {
	setTestAWSEnv(t, "")
	sess := openForTest(t, Settings{})
	if sess.Config().HTTPClient != nil {
		t.Fatal("Open set an HTTP client")
	}
	if aws.ToString(sess.Config().BaseEndpoint) != "" {
		t.Fatalf("base endpoint %q, want empty", aws.ToString(sess.Config().BaseEndpoint))
	}
	creds, err := sess.Config().Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("Retrieve: unexpected error: %v", err)
	}
	if creds.AccessKeyID != testAccessKey {
		if creds.AccessKeyID == fileAccessKey {
			t.Fatal("the credential chain used the shared credentials file")
		}
		t.Fatal("the credential chain returned an unexpected access key")
	}
	if creds.SecretAccessKey != testSecret {
		t.Fatal("the secret does not match the env value")
	}
	if creds.SessionToken != testToken {
		t.Fatal("the session token does not match the env value")
	}
	if sess.Config().Region != testRegion {
		t.Fatalf("region %q, want %q", sess.Config().Region, testRegion)
	}
}

func TestConfigCopyKeepsAllowlist(t *testing.T) {
	setTestAWSEnv(t, "")
	client := &denyClient{}
	sess := openForTest(t, Settings{HTTPClient: client})
	cfg := sess.Config()
	cfg.APIOptions = nil
	_, err := sts.NewFromConfig(sess.Config()).AssumeRole(context.Background(), &sts.AssumeRoleInput{})
	if !errors.Is(err, ErrNotReadOnly) {
		t.Fatalf("AssumeRole error %v does not wrap %v", err, ErrNotReadOnly)
	}
	if client.calls.Load() != 0 {
		t.Fatal("AssumeRole made a network call")
	}
}

func TestReadOnlyMiddlewareIsFirst(t *testing.T) {
	stack := middleware.NewStack("test", func() interface{} { return nil })
	if err := stack.Initialize.Add(markMiddleware{id: "later"}, middleware.After); err != nil {
		t.Fatal(err)
	}
	if err := addReadOnly(nil)(stack); err != nil {
		t.Fatal(err)
	}
	ids := stack.Initialize.List()
	if len(ids) < 2 || ids[0] != (readOnly{}).ID() {
		t.Fatalf("initialize order %v, want ReadOnlyAllowlist first", ids)
	}
}

type markMiddleware struct {
	id string
}

func (m markMiddleware) ID() string { return m.id }

func (m markMiddleware) HandleInitialize(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
	return next.HandleInitialize(ctx, in)
}

func TestIAMPolicyListsOnlyReadActions(t *testing.T) {
	raw := policyJSON(t)
	var doc struct {
		Version   string `json:"Version"`
		Statement []struct {
			Effect    string          `json:"Effect"`
			Action    json.RawMessage `json:"Action"`
			NotAction json.RawMessage `json:"NotAction"`
			Resource  json.RawMessage `json:"Resource"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("policy JSON: %v", err)
	}
	if doc.Version != "2012-10-17" {
		t.Fatalf("policy version %q, want %q", doc.Version, "2012-10-17")
	}
	if len(doc.Statement) == 0 {
		t.Fatal("policy has no statement")
	}
	var actions []string
	for i, stmt := range doc.Statement {
		if stmt.Effect != "Allow" {
			t.Fatalf("statement %d effect %q, want Allow", i, stmt.Effect)
		}
		if len(stmt.NotAction) != 0 && string(stmt.NotAction) != "null" {
			t.Fatalf("statement %d sets NotAction", i)
		}
		resource := strings.Trim(string(stmt.Resource), `"`)
		if resource != "*" {
			t.Fatalf("statement %d resource %s, want *", i, stmt.Resource)
		}
		actions = append(actions, decodeActions(t, stmt.Action)...)
	}
	if len(actions) == 0 {
		t.Fatal("policy has no action")
	}
	seen := map[string]struct{}{}
	for _, action := range actions {
		if _, ok := seen[action]; ok {
			t.Fatalf("duplicate action %s", action)
		}
		seen[action] = struct{}{}
		if !readAction(action) {
			t.Fatalf("action %s is not a read action", action)
		}
	}
	if _, ok := seen["sts:GetCallerIdentity"]; !ok {
		t.Fatal("policy does not allow sts:GetCallerIdentity")
	}
	if _, ok := seen["s3:GetObject"]; ok {
		t.Fatal("policy allows s3:GetObject")
	}
}

func TestDependenciesStayInTheAllowList(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on PATH")
	}
	cmd := exec.Command(goTool, "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "github.com/itstoni0/swale/internal/awsread")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out)
	}
	forbidden := []string{
		"github.com/aws/aws-sdk-go-v2/service/ec2",
		"github.com/aws/aws-sdk-go-v2/service/s3",
		"github.com/aws/aws-sdk-go-v2/service/iam",
		"github.com/aws/aws-sdk-go-v2/service/ecs",
		"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2",
		"github.com/aws/aws-sdk-go-v2/service/rds",
	}
	deps := string(out)
	for _, path := range forbidden {
		if strings.Contains(deps, path) {
			t.Errorf("forbidden dependency: %s", path)
		}
	}
}

func testLogger(buf *bytes.Buffer) *log.Logger {
	return log.New(buf, "", 0)
}

func runOperation(t *testing.T, name string, logBuf *bytes.Buffer, fn middleware.HandlerFunc) error {
	t.Helper()
	var logger *log.Logger
	if logBuf != nil {
		logger = testLogger(logBuf)
	}
	stack := middleware.NewStack(name, func() interface{} { return nil })
	if err := addReadOnly(logger)(stack); err != nil {
		t.Fatalf("add middleware: %v", err)
	}
	ctx := context.Background()
	if name != "" {
		ctx = middleware.WithOperationName(ctx, name)
	}
	_, _, err := stack.HandleMiddleware(ctx, struct{}{}, fn)
	return err
}

func policyJSON(t *testing.T) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("the test file path is not available")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "docs", "iam-read-only-policy.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const start = "```json\n"
	text := string(data)
	i := strings.Index(text, start)
	if i < 0 {
		t.Fatal("the policy doc has no json block")
	}
	rest := text[i+len(start):]
	j := strings.Index(rest, "\n```")
	if j < 0 {
		t.Fatal("the policy doc json block does not end")
	}
	return []byte(rest[:j])
}

func decodeActions(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		t.Fatalf("action %s: %v", raw, err)
	}
	return many
}

func readAction(action string) bool {
	service, verb, ok := strings.Cut(action, ":")
	if !ok || service == "" || strings.Contains(service, "*") {
		return false
	}
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(verb, prefix) {
			rest := strings.TrimPrefix(verb, prefix)
			if rest == "" || rest == "*" {
				return true
			}
			if strings.HasSuffix(rest, "*") {
				rest = strings.TrimSuffix(rest, "*")
			}
			if rest == "" || strings.ContainsAny(rest, "*?:") {
				return false
			}
			return true
		}
	}
	return false
}

func assertNoSecrets(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{testAccessKey, testSecret, testToken, fileAccessKey, "file-secret-not-used", "file-token-not-used"} {
		if strings.Contains(text, secret) {
			t.Fatalf("log contains a credential value %q", secret)
		}
	}
}
