package inventory_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/itstoni0/swale/internal/inventory"
)

const modulePath = "github.com/itstoni0/swale"

// sampleInventory returns an inventory that sets every field.
// It uses each managed status and one redacted value.
func sampleInventory() inventory.Inventory {
	return inventory.Inventory{
		Resources: []inventory.Resource{
			{
				Type:    "aws_vpc",
				ID:      "vpc-0a1b2c3d",
				ARN:     "arn:aws:ec2:eu-west-1:111122223333:vpc/vpc-0a1b2c3d",
				Account: "111122223333",
				Region:  "eu-west-1",
				Managed: inventory.StatusManaged,
				Tags: map[string]string{
					"Name":        "main",
					"Environment": "prod",
				},
				Attributes: map[string]string{
					"cidr_block":           "10.0.0.0/16",
					"enable_dns_hostnames": "true",
				},
			},
			{
				Type:    "aws_subnet",
				ID:      "subnet-0e1f2a3b",
				ARN:     "arn:aws:ec2:eu-west-1:111122223333:subnet/subnet-0e1f2a3b",
				Account: "111122223333",
				Region:  "eu-west-1",
				Managed: inventory.StatusUnmanaged,
				Tags: map[string]string{
					"Name": "private-a",
				},
				Attributes: map[string]string{
					"cidr_block":        "10.0.1.0/24",
					"availability_zone": "eu-west-1a",
				},
				Links: []inventory.Link{
					{Relation: "member_of", TargetType: "aws_vpc", TargetID: "vpc-0a1b2c3d"},
				},
			},
			{
				Type:    "aws_db_instance",
				ID:      "orders-db",
				ARN:     "arn:aws:rds:eu-west-1:111122223333:db:orders-db",
				Account: "111122223333",
				Region:  "eu-west-1",
				Managed: inventory.StatusUnknown,
				Tags: map[string]string{
					"Team": "orders",
				},
				Attributes: map[string]string{
					"engine":          "postgres",
					"master_password": inventory.Redacted,
				},
				Links: []inventory.Link{
					{Relation: "member_of", TargetType: "aws_vpc", TargetID: "vpc-0a1b2c3d"},
					{Relation: "uses", TargetType: "aws_subnet", TargetID: "subnet-0e1f2a3b"},
				},
			},
		},
	}
}

// sampleJSON is the wire form of sampleInventory.
// It fixes the JSON field names and the managed status values.
const sampleJSON = `{
  "resources": [
    {
      "type": "aws_vpc",
      "id": "vpc-0a1b2c3d",
      "arn": "arn:aws:ec2:eu-west-1:111122223333:vpc/vpc-0a1b2c3d",
      "account": "111122223333",
      "region": "eu-west-1",
      "managed": "managed",
      "tags": {"Name": "main", "Environment": "prod"},
      "attributes": {"cidr_block": "10.0.0.0/16", "enable_dns_hostnames": "true"}
    },
    {
      "type": "aws_subnet",
      "id": "subnet-0e1f2a3b",
      "arn": "arn:aws:ec2:eu-west-1:111122223333:subnet/subnet-0e1f2a3b",
      "account": "111122223333",
      "region": "eu-west-1",
      "managed": "unmanaged",
      "tags": {"Name": "private-a"},
      "attributes": {"cidr_block": "10.0.1.0/24", "availability_zone": "eu-west-1a"},
      "links": [
        {"relation": "member_of", "target_type": "aws_vpc", "target_id": "vpc-0a1b2c3d"}
      ]
    },
    {
      "type": "aws_db_instance",
      "id": "orders-db",
      "arn": "arn:aws:rds:eu-west-1:111122223333:db:orders-db",
      "account": "111122223333",
      "region": "eu-west-1",
      "managed": "unknown",
      "tags": {"Team": "orders"},
      "attributes": {"engine": "postgres", "master_password": "[REDACTED]"},
      "links": [
        {"relation": "member_of", "target_type": "aws_vpc", "target_id": "vpc-0a1b2c3d"},
        {"relation": "uses", "target_type": "aws_subnet", "target_id": "subnet-0e1f2a3b"}
      ]
    }
  ]
}`

func encode(t *testing.T, inv inventory.Inventory) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := inventory.Encode(&buf, inv); err != nil {
		t.Fatalf("Encode: unexpected error: %v", err)
	}
	return buf.Bytes()
}

func TestRoundTripFullSample(t *testing.T) {
	want := sampleInventory()

	data := encode(t, want)
	got, err := inventory.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v\nJSON:\n%s", err, data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the inventory\ngot:  %#v\nwant: %#v\nJSON:\n%s", got, want, data)
	}
}

func TestRoundTripMinimalResource(t *testing.T) {
	want := inventory.Inventory{
		Resources: []inventory.Resource{
			{Type: "aws_s3_bucket", ID: "logs-bucket", Managed: inventory.StatusUnknown},
		},
	}

	data := encode(t, want)
	got, err := inventory.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v\nJSON:\n%s", err, data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the inventory\ngot:  %#v\nwant: %#v\nJSON:\n%s", got, want, data)
	}
}

func TestDecodeWireFormat(t *testing.T) {
	got, err := inventory.Decode(strings.NewReader(sampleJSON))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if want := sampleInventory(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Decode gave a different inventory\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestRedactedMarker(t *testing.T) {
	if inventory.Redacted != "[REDACTED]" {
		t.Fatalf("Redacted = %q, want %q", inventory.Redacted, "[REDACTED]")
	}

	data := encode(t, sampleInventory())
	if !bytes.Contains(data, []byte(`"master_password":"[REDACTED]"`)) &&
		!bytes.Contains(data, []byte(`"master_password": "[REDACTED]"`)) {
		t.Fatalf("encoded JSON does not hold the redacted marker for master_password:\n%s", data)
	}
}

func TestManagedStatusValues(t *testing.T) {
	cases := map[inventory.ManagedStatus]string{
		inventory.StatusManaged:   "managed",
		inventory.StatusUnmanaged: "unmanaged",
		inventory.StatusUnknown:   "unknown",
	}
	for status, want := range cases {
		if string(status) != want {
			t.Errorf("status %q, want %q", status, want)
		}
	}
}

func TestDecodeRefusesInvalidResource(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr error
		index   string
	}{
		{
			name:    "missing type",
			json:    `{"resources":[{"id":"vpc-1","managed":"managed"}]}`,
			wantErr: inventory.ErrMissingType,
			index:   "resources[0]",
		},
		{
			name:    "empty type",
			json:    `{"resources":[{"type":"","id":"vpc-1","managed":"managed"}]}`,
			wantErr: inventory.ErrMissingType,
			index:   "resources[0]",
		},
		{
			name:    "missing id",
			json:    `{"resources":[{"type":"aws_vpc","managed":"managed"}]}`,
			wantErr: inventory.ErrMissingID,
			index:   "resources[0]",
		},
		{
			name:    "empty id",
			json:    `{"resources":[{"type":"aws_vpc","id":"","managed":"managed"}]}`,
			wantErr: inventory.ErrMissingID,
			index:   "resources[0]",
		},
		{
			name: "missing type in a later resource",
			json: `{"resources":[
				{"type":"aws_vpc","id":"vpc-1","managed":"managed"},
				{"type":"aws_subnet","id":"subnet-1","managed":"unmanaged"},
				{"id":"sg-1","managed":"unknown"}
			]}`,
			wantErr: inventory.ErrMissingType,
			index:   "resources[2]",
		},
		{
			name: "missing id in a later resource",
			json: `{"resources":[
				{"type":"aws_vpc","id":"vpc-1","managed":"managed"},
				{"type":"aws_subnet","managed":"unmanaged"}
			]}`,
			wantErr: inventory.ErrMissingID,
			index:   "resources[1]",
		},
		{
			name:    "invalid managed status",
			json:    `{"resources":[{"type":"aws_vpc","id":"vpc-1","managed":"maybe"}]}`,
			wantErr: inventory.ErrInvalidStatus,
			index:   "resources[0]",
		},
		{
			name:    "managed status in the wrong case",
			json:    `{"resources":[{"type":"aws_vpc","id":"vpc-1","managed":"Managed"}]}`,
			wantErr: inventory.ErrInvalidStatus,
			index:   "resources[0]",
		},
		{
			name:    "missing managed status",
			json:    `{"resources":[{"type":"aws_vpc","id":"vpc-1"}]}`,
			wantErr: inventory.ErrInvalidStatus,
			index:   "resources[0]",
		},
		{
			name: "invalid managed status in a later resource",
			json: `{"resources":[
				{"type":"aws_vpc","id":"vpc-1","managed":"managed"},
				{"type":"aws_subnet","id":"subnet-1","managed":"yes"}
			]}`,
			wantErr: inventory.ErrInvalidStatus,
			index:   "resources[1]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := inventory.Decode(strings.NewReader(tc.json))
			if err == nil {
				t.Fatalf("Decode: got no error and %#v, want %v", got, tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Decode: error %q does not wrap %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.index) {
				t.Fatalf("Decode: error %q does not name %s", err, tc.index)
			}
			if !reflect.DeepEqual(got, inventory.Inventory{}) {
				t.Fatalf("Decode: got %#v with the error, want the zero Inventory", got)
			}
		})
	}
}

func TestDecodeRefusesMalformedJSON(t *testing.T) {
	cases := map[string]string{
		"empty input":     ``,
		"truncated":       `{"resources":[{"type":"aws_vpc"`,
		"wrong top level": `[]`,
		"wrong field":     `{"resources":{"type":"aws_vpc"}}`,
		"second value":    `{"resources":[]} {"resources":[]}`,
		"trailing text":   `{"resources":[]} x`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := inventory.Decode(strings.NewReader(input)); err == nil {
				t.Fatalf("Decode(%q): got no error, want an error", input)
			}
		})
	}
}

func TestEmptyInventory(t *testing.T) {
	data := encode(t, inventory.Inventory{})

	var wire struct {
		Resources json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("encoded JSON is not valid: %v\n%s", err, data)
	}
	if got := string(bytes.TrimSpace(wire.Resources)); got != "[]" {
		t.Fatalf("resources encodes as %q, want %q\nJSON:\n%s", got, "[]", data)
	}

	got, err := inventory.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(got.Resources) != 0 {
		t.Fatalf("Decode: got %d resources, want 0", len(got.Resources))
	}
}

func TestDecodeEmptyResourceList(t *testing.T) {
	got, err := inventory.Decode(strings.NewReader(`{"resources":[]}`))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(got.Resources) != 0 {
		t.Fatalf("Decode: got %d resources, want 0", len(got.Resources))
	}
}

// Decode accepts white space after the JSON value.
func TestDecodeAcceptsTrailingWhitespace(t *testing.T) {
	got, err := inventory.Decode(strings.NewReader("{\"resources\":[]}\n  \n"))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(got.Resources) != 0 {
		t.Fatalf("Decode: got %d resources, want 0", len(got.Resources))
	}
}

func TestStandardLibraryOnly(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on PATH")
	}

	pkg := modulePath + "/internal/inventory"
	cmd := exec.Command(goTool, "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", pkg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out)
	}

	for _, dep := range strings.Fields(string(out)) {
		if dep != modulePath && !strings.HasPrefix(dep, modulePath+"/") {
			t.Errorf("non-stdlib dependency: %s", dep)
		}
	}
}
