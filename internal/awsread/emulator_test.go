//go:build emulator

package awsread

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestEmulatorCaller reads the account ID and the Region from AWS Emulator.
// The emulator account is 000000000000.
func TestEmulatorCaller(t *testing.T) {
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		t.Fatal("AWS_ENDPOINT_URL is empty")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		t.Fatal("AWS_REGION is empty")
	}
	if os.Getenv("AWS_CA_BUNDLE") == "" {
		t.Fatal("AWS_CA_BUNDLE is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := Open(ctx, Settings{})
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	got, err := sess.Caller(ctx)
	if err != nil {
		t.Fatalf("Caller: unexpected error: %v", err)
	}
	if got.AccountID != "000000000000" {
		t.Fatalf("account ID %q, want %q", got.AccountID, "000000000000")
	}
	if got.Region != region {
		t.Fatalf("region %q, want %q", got.Region, region)
	}
}
