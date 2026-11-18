// Package awsread calls AWS read APIs for discovery.
// The package uses the AWS SDK for Go v2.
// The package does not run the AWS CLI.
// A client from a Session refuses an operation that is not read-only.
// The file docs/iam-read-only-policy.md gives the IAM policy for discovery.
package awsread

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/logging"
	"github.com/aws/smithy-go/middleware"
)

// ErrNotReadOnly is the error for an operation that is not read-only.
var ErrNotReadOnly = errors.New("operation is not read-only")

// readOnlyPrefixes are the allowed operation name prefixes.
// An operation name must start with one prefix.
var readOnlyPrefixes = []string{"Describe", "List", "Get"}

// blockedOperations start with Get.
// Each operation returns a credential.
// A credential is not a read.
// The allowlist does not include these names.
var blockedOperations = []string{
	"GetSessionToken",
	"GetFederationToken",
	"GetDelegatedAccessToken",
	"GetWebIdentityToken",
}

// Caller is the account ID and the Region for one discovery session.
// The account ID comes from sts:GetCallerIdentity.
// The Region comes from the AWS config for that call.
type Caller struct {
	AccountID string
	Region    string
}

// Settings controls Open.
// A zero Settings value uses the standard AWS config.
type Settings struct {
	// Load reads the AWS config.
	// A nil Load calls config.LoadDefaultConfig.
	Load func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error)

	// HTTPClient sends each AWS call.
	// A nil HTTPClient keeps the client from Load.
	// The SDK client honors AWS_CA_BUNDLE.
	// A set HTTPClient replaces that client.
	HTTPClient aws.HTTPClient

	// Logger receives safe log lines.
	// A nil Logger discards log lines.
	Logger *log.Logger
}

// Session is a read-only AWS session for discovery.
type Session struct {
	cfg    aws.Config
	logger *log.Logger
}

// Open loads the AWS config and attaches the read-only check.
// Open uses the standard credential chain.
// Open does not read a credential file directly.
// LoadDefaultConfig honors AWS_ENDPOINT_URL.
func Open(ctx context.Context, settings Settings) (*Session, error) {
	load := settings.Load
	if load == nil {
		load = config.LoadDefaultConfig
	}
	cfg, err := load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	if settings.HTTPClient != nil {
		cfg.HTTPClient = settings.HTTPClient
	}
	// The SDK logger can write a request header.
	// A request header can hold a credential or a signature.
	// This session does not use the SDK logger.
	cfg.Logger = logging.Nop{}
	cfg.ClientLogMode = 0
	if cfg.Region == "" {
		return nil, errors.New("load aws config: region is empty")
	}
	cfg.APIOptions = append(cfg.APIOptions, addReadOnly(settings.Logger))
	return &Session{cfg: cfg, logger: settings.Logger}, nil
}

// Config returns the AWS config for a discovery client.
// A service client from this config refuses an operation that is not read-only.
func (s *Session) Config() aws.Config {
	cfg := s.cfg
	cfg.APIOptions = append([]func(*middleware.Stack) error{}, s.cfg.APIOptions...)
	return cfg
}

// Caller records the account ID and the Region.
// The account ID comes from sts:GetCallerIdentity.
func (s *Session) Caller(ctx context.Context) (Caller, error) {
	client := sts.NewFromConfig(s.cfg)
	out, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		logf(s.logger, "get caller identity failed")
		return Caller{}, fmt.Errorf("get caller identity: %w", err)
	}
	account := strings.TrimSpace(aws.ToString(out.Account))
	if account == "" {
		logf(s.logger, "get caller identity failed")
		return Caller{}, errors.New("get caller identity: account ID is empty")
	}
	caller := Caller{AccountID: account, Region: s.cfg.Region}
	logf(s.logger, "caller account %s region %s", caller.AccountID, caller.Region)
	return caller, nil
}

// readOnly refuses an operation that is not read-only.
type readOnly struct {
	logger *log.Logger
}

func (readOnly) ID() string { return "ReadOnlyAllowlist" }

// HandleInitialize refuses the operation before the signature step and before the HTTP call.
func (r readOnly) HandleInitialize(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
	name := middleware.GetOperationName(ctx)
	if !allowedOperation(name) {
		logf(r.logger, "refused operation %s", name)
		return middleware.InitializeOutput{}, middleware.Metadata{}, fmt.Errorf("%w: %s", ErrNotReadOnly, name)
	}
	return next.HandleInitialize(ctx, in)
}

// addReadOnly attaches the read-only check to a client stack.
// The Initialize step runs before the signature step and before the HTTP call.
func addReadOnly(logger *log.Logger) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Initialize.Add(readOnly{logger: logger}, middleware.Before)
	}
}

// allowedOperation reports whether the operation name is read-only.
func allowedOperation(name string) bool {
	for _, blocked := range blockedOperations {
		if name == blocked {
			return false
		}
	}
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func logf(logger *log.Logger, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Printf(format, args...)
}
