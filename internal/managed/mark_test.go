package managed_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/itstoni0/swale/internal/inventory"
	"github.com/itstoni0/swale/internal/managed"
)

const modulePath = "github.com/itstoni0/swale"

const (
	vpcID     = "vpc-sample"
	vpcARN    = "arn:aws:ec2:eu-west-1:example:vpc/vpc-sample"
	subnetID  = "subnet-sample"
	subnetARN = "arn:aws:ec2:eu-west-1:example:subnet/subnet-sample"
	sgARN     = "arn:aws:ec2:eu-west-1:example:security-group/sg-sample"
	dataID    = "data-caller-example"
	dataARN   = "arn:aws:iam::example:root"
	otherID   = "subnet-other"
)

var accountID = regexp.MustCompile(`[0-9]{12}`)

// recordedCall is one command that a fake runner observed.
type recordedCall struct {
	dir  string
	name string
	args []string
}

// fakeRunner returns fixture output and records every command.
type fakeRunner struct {
	calls []recordedCall
	out   map[string][]byte
	errs  map[string]error
}

func commandKey(dir, name string, args ...string) string {
	parts := make([]string, 0, 2+len(args))
	parts = append(parts, dir, name)
	parts = append(parts, args...)
	return strings.Join(parts, "\x00")
}

func (f *fakeRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	copied := append([]string(nil), args...)
	f.calls = append(f.calls, recordedCall{dir: dir, name: name, args: copied})
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	key := commandKey(dir, name, args...)
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if out, ok := f.out[key]; ok {
		return append([]byte(nil), out...), nil
	}
	return nil, errors.New("unexpected command " + name + " " + strings.Join(args, " ") + " in " + dir)
}

func commandLine(name string, args []string) string {
	base := filepath.Base(name)
	if len(args) == 0 {
		return base
	}
	return base + " " + strings.Join(args, " ")
}

// assertReadOnly fails when a call is outside the read-only command list.
func assertReadOnly(t *testing.T, calls []recordedCall) {
	t.Helper()
	allowed := map[string]bool{
		"terragrunt find --json": true,
		"terragrunt show -json":  true,
		"terragrunt state pull":  true,
		"terraform show -json":   true,
		"terraform state pull":   true,
	}
	for _, call := range calls {
		line := commandLine(call.name, call.args)
		if !allowed[line] {
			t.Fatalf("command %q is not read-only", line)
		}
	}
}

func sampleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("the test file path is not available")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "sample-repo")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("sample repository path: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("sample repository: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("sample repository %s is not a directory", abs)
	}
	return abs
}

func readRepoFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return data
}

func sampleRunner(t *testing.T, root string) *fakeRunner {
	t.Helper()
	return &fakeRunner{
		out: map[string][]byte{
			commandKey(root, "terragrunt", "find", "--json"):                                 readRepoFile(t, root, filepath.Join("recorded", "find.json")),
			commandKey(filepath.Join(root, "live", "vpc"), "terragrunt", "show", "-json"):    readRepoFile(t, root, filepath.Join("recorded", "live", "vpc", "show.json")),
			commandKey(filepath.Join(root, "live", "subnet"), "terragrunt", "show", "-json"): readRepoFile(t, root, filepath.Join("recorded", "live", "subnet", "show.json")),
		},
	}
}

func baseResource(kind, id, arn string, status inventory.ManagedStatus) inventory.Resource {
	return inventory.Resource{
		Type:       kind,
		ID:         id,
		ARN:        arn,
		Region:     "eu-west-1",
		Managed:    status,
		Tags:       map[string]string{"Name": id},
		Attributes: map[string]string{"origin": "inventory"},
	}
}

func samplePair() (inventory.Inventory, inventory.Inventory) {
	vpc := baseResource("aws_vpc", vpcID, vpcARN, inventory.StatusUnmanaged)
	subnet := baseResource("aws_subnet", subnetID, subnetARN, inventory.StatusUnmanaged)
	subnet.Links = []inventory.Link{{
		Relation:   "member_of",
		TargetType: "aws_vpc",
		TargetID:   vpcID,
	}}
	byARN := baseResource("aws_security_group", "sg-not-the-id", sgARN, inventory.StatusUnmanaged)
	idIsARN := baseResource("aws_vpc", vpcARN, "", inventory.StatusUnknown)
	other := baseResource("aws_subnet", otherID, "arn:aws:ec2:eu-west-1:example:subnet/subnet-other", inventory.StatusManaged)
	dataRes := baseResource("aws_caller_identity", dataID, dataARN, inventory.StatusManaged)

	in := inventory.Inventory{Resources: []inventory.Resource{vpc, subnet, byARN, idIsARN, other, dataRes}}
	want := inventory.Inventory{Resources: append([]inventory.Resource(nil), in.Resources...)}
	want.Resources[0].Managed = inventory.StatusManaged
	want.Resources[1].Managed = inventory.StatusManaged
	want.Resources[2].Managed = inventory.StatusManaged
	want.Resources[3].Managed = inventory.StatusManaged
	want.Resources[4].Managed = inventory.StatusUnmanaged
	want.Resources[5].Managed = inventory.StatusUnmanaged
	return in, want
}

func markSample(t *testing.T) (inventory.Inventory, *fakeRunner) {
	t.Helper()
	root := sampleRoot(t)
	fake := sampleRunner(t, root)
	in, _ := samplePair()
	snapshot := append([]inventory.Resource(nil), in.Resources...)
	got, err := managed.Mark(context.Background(), fake, root, in)
	if err != nil {
		t.Fatalf("Mark: %v\ncalls: %#v", err, fake.calls)
	}
	if !reflect.DeepEqual(in.Resources, snapshot) {
		t.Fatalf("Mark changed the input\ngot:  %#v\nwant: %#v", in.Resources, snapshot)
	}
	assertReadOnly(t, fake.calls)
	return got, fake
}

func TestSampleRepositoryMarksManagedResources(t *testing.T) {
	got, _ := markSample(t)
	_, want := samplePair()
	if len(got.Resources) != len(want.Resources) {
		t.Fatalf("got %d resources, want %d", len(got.Resources), len(want.Resources))
	}
	for i := range want.Resources {
		if !reflect.DeepEqual(got.Resources[i], want.Resources[i]) {
			t.Fatalf("resource %d\ngot:  %#v\nwant: %#v", i, got.Resources[i], want.Resources[i])
		}
	}
}

func TestUnmatchedResourceIsUnmanaged(t *testing.T) {
	got, _ := markSample(t)
	var found bool
	for _, res := range got.Resources {
		if res.ID != otherID {
			continue
		}
		found = true
		if res.Managed != inventory.StatusUnmanaged {
			t.Fatalf("resource %s is %s, want unmanaged", otherID, res.Managed)
		}
	}
	if !found {
		t.Fatalf("resource %s is absent", otherID)
	}
}

func TestMarkNeverCallsStateWrite(t *testing.T) {
	_, fake := markSample(t)
	if len(fake.calls) == 0 {
		t.Fatal("Mark recorded no command")
	}
	root := sampleRoot(t)
	want := []recordedCall{
		{dir: root, name: "terragrunt", args: []string{"find", "--json"}},
		{dir: filepath.Join(root, "live", "vpc"), name: "terragrunt", args: []string{"show", "-json"}},
		{dir: filepath.Join(root, "live", "subnet"), name: "terragrunt", args: []string{"show", "-json"}},
	}
	if len(fake.calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", fake.calls, want)
	}
	for i := range want {
		if fake.calls[i].dir != want[i].dir || fake.calls[i].name != want[i].name || !reflect.DeepEqual(fake.calls[i].args, want[i].args) {
			t.Fatalf("call %d = %#v, want %#v", i, fake.calls[i], want[i])
		}
	}
	writes := []string{"apply", "destroy", "import", "taint", "untaint", "refresh", "force-unlock", "rm", "mv", "push"}
	for _, call := range fake.calls {
		line := commandLine(call.name, call.args)
		for _, write := range writes {
			if strings.Contains(line, write) {
				t.Fatalf("command %q writes state", line)
			}
		}
	}
}

func TestSampleRepoMatchesFindFixture(t *testing.T) {
	root := sampleRoot(t)
	var items []struct {
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(readRepoFile(t, root, filepath.Join("recorded", "find.json")), &items); err != nil {
		t.Fatalf("find JSON: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("find JSON has no item")
	}
	for _, item := range items {
		var name string
		switch item.Type {
		case "unit":
			name = "terragrunt.hcl"
		case "stack":
			name = "terragrunt.stack.hcl"
		default:
			t.Fatalf("type %q is not a unit or a stack", item.Type)
		}
		path := filepath.Join(root, filepath.FromSlash(item.Path), name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("fixture %s: %v", path, err)
		}
		if item.Type == "unit" {
			showPath := filepath.Join(root, "recorded", filepath.FromSlash(item.Path), "show.json")
			if _, err := os.Stat(showPath); err != nil {
				t.Fatalf("state fixture %s: %v", showPath, err)
			}
		}
	}
}

func TestSampleRepoHasNoAccountData(t *testing.T) {
	root := sampleRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".env" || strings.HasPrefix(d.Name(), ".env.") {
			t.Errorf("unexpected env file %s", path)
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if accountID.Match(data) {
			t.Errorf("%s contains a 12 digit number", path)
		}
		lower := strings.ToLower(string(data))
		for _, secret := range []string{"akia", "asia", "aws_secret_access_key"} {
			if strings.Contains(lower, secret) {
				t.Errorf("%s contains %s", path, secret)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnknownWhenShowCommandFails(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good")
	bad := filepath.Join(root, "bad")
	showErr := errors.New("backend down")
	findJSON := `[{"type":"unit","path":"good"},{"type":"unit","path":"bad"}]`
	showJSON := `{"values":{"root_module":{"resources":[{"mode":"managed","values":{"id":"vpc-sample"}}]}}}`
	fake := &fakeRunner{
		out: map[string][]byte{
			commandKey(root, "terragrunt", "find", "--json"): []byte(findJSON),
			commandKey(good, "terragrunt", "show", "-json"):  []byte(showJSON),
		},
		errs: map[string]error{
			commandKey(bad, "terragrunt", "show", "-json"): showErr,
		},
	}
	in := inventory.Inventory{Resources: []inventory.Resource{
		{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusUnmanaged, Tags: map[string]string{"Name": "sample"}},
		{Type: "aws_subnet", ID: "subnet-other", Managed: inventory.StatusManaged},
		{Type: "aws_vpc", Managed: inventory.StatusManaged},
	}}
	snapshot := append([]inventory.Resource(nil), in.Resources...)
	got, err := managed.Mark(context.Background(), fake, root, in)
	if !errors.Is(err, managed.ErrStateUnread) {
		t.Fatalf("error %v, want %v", err, managed.ErrStateUnread)
	}
	if !errors.Is(err, showErr) {
		t.Fatalf("error %v does not wrap %v", err, showErr)
	}
	if !strings.Contains(err.Error(), bad) {
		t.Fatalf("error %q does not name %s", err, bad)
	}
	if !reflect.DeepEqual(in.Resources, snapshot) {
		t.Fatal("Mark changed the input")
	}
	if got.Resources[0].Managed != inventory.StatusManaged || got.Resources[0].Tags["Name"] != "sample" {
		t.Fatalf("matched resource = %#v", got.Resources[0])
	}
	if got.Resources[1].Managed != inventory.StatusUnknown {
		t.Fatalf("unmatched resource is %s, want unknown", got.Resources[1].Managed)
	}
	if got.Resources[2].Managed != inventory.StatusUnknown {
		t.Fatalf("blank resource is %s, want unknown", got.Resources[2].Managed)
	}
	assertReadOnly(t, fake.calls)
}

func TestUnknownWhenShowJSONIsInvalid(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		out  string
	}{
		{name: "empty", out: " \n"},
		{name: "invalid", out: "{"},
		{name: "array", out: "[]"},
		{name: "state pull", out: `{"version":4,"resources":[{"mode":"managed","type":"aws_vpc","instances":[{"attributes":{"id":"vpc-sample"}}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRunner{out: map[string][]byte{
				commandKey(root, "terragrunt", "find", "--json"): []byte(`[{"type":"unit","path":"."}]`),
				commandKey(root, "terragrunt", "show", "-json"):  []byte(tc.out),
			}}
			in := inventory.Inventory{Resources: []inventory.Resource{
				{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusUnmanaged},
			}}
			got, err := managed.Mark(context.Background(), fake, root, in)
			if !errors.Is(err, managed.ErrStateUnread) {
				t.Fatalf("error %v, want %v", err, managed.ErrStateUnread)
			}
			if got.Resources[0].Managed != inventory.StatusUnknown {
				t.Fatalf("status = %s, want unknown", got.Resources[0].Managed)
			}
			assertReadOnly(t, fake.calls)
		})
	}
}

func TestEmptyStateIsUnmanaged(t *testing.T) {
	root := t.TempDir()
	fake := &fakeRunner{out: map[string][]byte{
		commandKey(root, "terragrunt", "find", "--json"): []byte(`[{"type":"unit","path":"."}]`),
		commandKey(root, "terragrunt", "show", "-json"):  []byte(`{"values":{}}`),
	}}
	in := inventory.Inventory{Resources: []inventory.Resource{
		{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusManaged},
	}}
	got, err := managed.Mark(context.Background(), fake, root, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resources[0].Managed != inventory.StatusUnmanaged {
		t.Fatalf("status = %s, want unmanaged", got.Resources[0].Managed)
	}
	assertReadOnly(t, fake.calls)
}

func TestStateIdentityRules(t *testing.T) {
	root := t.TempDir()
	unit := filepath.Join(root, "unit")
	showJSON := `{
	  "values": {
	    "root_module": {
	      "resources": [
	        {"mode": "managed", "values": {"id": " vpc-trim ", "arn": " arn-trim "}},
	        {"mode": "managed", "values": {"id": 10, "arn": ""}},
	        {"mode": "managed", "values": {"id": "   ", "arn": null}},
	        {"mode": "data", "values": {"id": "data-only", "arn": "arn-data"}},
	        {"mode": "Managed", "values": {"id": "wrong-case"}},
	        {"mode": "managed", "values": null},
	        {"mode": "managed"}
	      ],
	      "child_modules": [
	        {
	          "resources": [
	            {"mode": "managed", "values": {"id": "child-id", "arn": "child-arn"}}
	          ],
	          "child_modules": [
	            {
	              "resources": [
	                {"mode": "managed", "values": {"arn": "nested-arn"}}
	              ]
	            }
	          ]
	        }
	      ]
	    }
	  }
	}`
	fake := &fakeRunner{out: map[string][]byte{
		commandKey(root, "terragrunt", "find", "--json"): []byte(`[{"type":"unit","path":"unit"}]`),
		commandKey(unit, "terragrunt", "show", "-json"):  []byte(showJSON),
	}}
	in := inventory.Inventory{Resources: []inventory.Resource{
		{Type: "aws_vpc", ID: "vpc-trim", Managed: inventory.StatusUnmanaged},
		{Type: "aws_vpc", ID: "other", ARN: "arn-trim", Managed: inventory.StatusUnmanaged},
		{Type: "aws_vpc", ID: "  vpc-trim  ", Managed: inventory.StatusUnmanaged},
		{Type: "aws_vpc", ID: "10", Managed: inventory.StatusManaged},
		{Type: "aws_vpc", ID: "data-only", ARN: "arn-data", Managed: inventory.StatusManaged},
		{Type: "aws_vpc", ID: "wrong-case", Managed: inventory.StatusManaged},
		{Type: "aws_vpc", ID: "child-id", Managed: inventory.StatusUnmanaged},
		{Type: "aws_vpc", ID: "nope", ARN: "nested-arn", Managed: inventory.StatusUnmanaged},
		{Type: "aws_vpc", ID: "   ", ARN: "  ", Managed: inventory.StatusManaged},
		{Type: "aws_vpc", ARN: "missing-arn", Managed: inventory.StatusManaged},
		{Type: "aws_vpc", ID: "absent", Managed: inventory.StatusManaged},
	}}
	want := []inventory.ManagedStatus{
		inventory.StatusManaged,
		inventory.StatusManaged,
		inventory.StatusManaged,
		inventory.StatusUnmanaged,
		inventory.StatusUnmanaged,
		inventory.StatusUnmanaged,
		inventory.StatusManaged,
		inventory.StatusManaged,
		inventory.StatusUnknown,
		inventory.StatusUnmanaged,
		inventory.StatusUnmanaged,
	}
	got, err := managed.Mark(context.Background(), fake, root, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Resources) != len(want) {
		t.Fatalf("got %d resources, want %d", len(got.Resources), len(want))
	}
	for i, status := range want {
		if got.Resources[i].Managed != status {
			t.Fatalf("resource %d ID %q ARN %q is %s, want %s", i, got.Resources[i].ID, got.Resources[i].ARN, got.Resources[i].Managed, status)
		}
	}
	assertReadOnly(t, fake.calls)
}

func TestFindOutputRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	absItem, err := json.Marshal([]map[string]string{{"type": "unit", "path": outside}})
	if err != nil {
		t.Fatal(err)
	}
	findErr := errors.New("find failed")
	cases := []struct {
		name    string
		out     []byte
		runErr  error
		snippet string
	}{
		{name: "command error", runErr: findErr, snippet: "find failed"},
		{name: "empty output", out: []byte(" \n"), snippet: "find JSON is empty"},
		{name: "invalid JSON", out: []byte("{"), snippet: "parse find JSON"},
		{name: "JSON object", out: []byte(`{}`), snippet: "parse find JSON"},
		{name: "absolute path", out: absItem, snippet: "escapes"},
		{name: "parent path", out: []byte(`[{"type":"unit","path":".."}]`), snippet: "escapes"},
		{name: "nested escape", out: []byte(`[{"type":"unit","path":"a/../../outside"}]`), snippet: "escapes"},
		{name: "empty path", out: []byte(`[{"type":"unit","path":" "}]`), snippet: "unit path is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRunner{
				out:  map[string][]byte{},
				errs: map[string]error{},
			}
			key := commandKey(root, "terragrunt", "find", "--json")
			if tc.runErr != nil {
				fake.errs[key] = tc.runErr
			} else {
				fake.out[key] = tc.out
			}
			in := inventory.Inventory{Resources: []inventory.Resource{
				{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusManaged, Tags: map[string]string{"Name": "keep"}},
			}}
			snapshot := append([]inventory.Resource(nil), in.Resources...)
			got, err := managed.Mark(context.Background(), fake, root, in)
			if !errors.Is(err, managed.ErrStateUnread) {
				t.Fatalf("error %v, want %v", err, managed.ErrStateUnread)
			}
			if tc.runErr != nil && !errors.Is(err, tc.runErr) {
				t.Fatalf("error %v does not wrap %v", err, tc.runErr)
			}
			if !strings.Contains(err.Error(), tc.snippet) {
				t.Fatalf("error %q does not contain %q", err, tc.snippet)
			}
			if !reflect.DeepEqual(in.Resources, snapshot) {
				t.Fatal("Mark changed the input")
			}
			if got.Resources[0].Managed != inventory.StatusUnknown || got.Resources[0].Tags["Name"] != "keep" {
				t.Fatalf("resource = %#v, want unknown and the original tag", got.Resources[0])
			}
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %#v, want one find command", fake.calls)
			}
			assertReadOnly(t, fake.calls)
		})
	}
}

func TestNoUnits(t *testing.T) {
	cases := map[string]string{
		"empty list": `[]`,
		"null":       `null`,
		"stack only": `[{"type":"stack","path":"live"}]`,
	}
	for name, findJSON := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			fake := &fakeRunner{out: map[string][]byte{
				commandKey(root, "terragrunt", "find", "--json"): []byte(findJSON),
			}}
			in := inventory.Inventory{Resources: []inventory.Resource{
				{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusManaged},
			}}
			got, err := managed.Mark(context.Background(), fake, root, in)
			if err != nil {
				t.Fatal(err)
			}
			if got.Resources[0].Managed != inventory.StatusUnmanaged {
				t.Fatalf("status = %s, want unmanaged", got.Resources[0].Managed)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %#v, want one find command", fake.calls)
			}
			assertReadOnly(t, fake.calls)
		})
	}
}

func TestNilResourceListStaysNil(t *testing.T) {
	root := t.TempDir()
	fake := &fakeRunner{out: map[string][]byte{
		commandKey(root, "terragrunt", "find", "--json"): []byte(`[]`),
	}}
	got, err := managed.Mark(context.Background(), fake, root, inventory.Inventory{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Resources != nil {
		t.Fatalf("resources = %#v, want nil", got.Resources)
	}
}

func TestDuplicateUnitIsReadOnce(t *testing.T) {
	root := t.TempDir()
	unit := filepath.Join(root, "live", "vpc")
	findJSON := `[
	  {"type":"unit","path":"live/vpc"},
	  {"type":"stack","path":"live"},
	  {"type":"unit","path":"live/../live/vpc"}
	]`
	showJSON := `{"values":{"root_module":{"resources":[{"mode":"managed","values":{"id":"vpc-sample"}}]}}}`
	fake := &fakeRunner{out: map[string][]byte{
		commandKey(root, "terragrunt", "find", "--json"): []byte(findJSON),
		commandKey(unit, "terragrunt", "show", "-json"):  []byte(showJSON),
	}}
	in := inventory.Inventory{Resources: []inventory.Resource{
		{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusUnmanaged},
	}}
	got, err := managed.Mark(context.Background(), fake, root, in)
	if err != nil {
		t.Fatalf("Mark: %v\ncalls: %#v", err, fake.calls)
	}
	if got.Resources[0].Managed != inventory.StatusManaged {
		t.Fatalf("status = %s, want managed", got.Resources[0].Managed)
	}
	shows := 0
	for _, call := range fake.calls {
		if commandLine(call.name, call.args) == "terragrunt show -json" {
			shows++
			if call.dir != unit {
				t.Fatalf("show dir = %s, want %s", call.dir, unit)
			}
		}
	}
	if shows != 1 {
		t.Fatalf("show calls = %d, want 1\ncalls: %#v", shows, fake.calls)
	}
	assertReadOnly(t, fake.calls)
}

func TestCleanRootPath(t *testing.T) {
	root := t.TempDir()
	dirty := root + string(filepath.Separator)
	fake := &fakeRunner{out: map[string][]byte{
		commandKey(root, "terragrunt", "find", "--json"): []byte(`[]`),
	}}
	in := inventory.Inventory{Resources: []inventory.Resource{
		{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusManaged},
	}}
	got, err := managed.Mark(context.Background(), fake, dirty, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resources[0].Managed != inventory.StatusUnmanaged {
		t.Fatalf("status = %s, want unmanaged", got.Resources[0].Managed)
	}
	if len(fake.calls) != 1 || fake.calls[0].dir != root {
		t.Fatalf("calls = %#v, want find in %s", fake.calls, root)
	}
}

func TestMarkRejectsBadInput(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		run  managed.Runner
		root string
		want error
	}{
		{name: "nil runner", ctx: context.Background(), root: root, want: managed.ErrNilRunner},
		{name: "nil context", run: &fakeRunner{}, root: root, want: managed.ErrNilContext},
		{name: "empty root", ctx: context.Background(), run: &fakeRunner{}, root: " \n", want: managed.ErrEmptyRoot},
		{name: "missing root", ctx: context.Background(), run: &fakeRunner{}, root: missing, want: os.ErrNotExist},
		{name: "file root", ctx: context.Background(), run: &fakeRunner{}, root: file, want: managed.ErrNotDirectory},
		{name: "canceled context", ctx: canceled, run: &fakeRunner{}, root: root, want: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := inventory.Inventory{Resources: []inventory.Resource{
				{Type: "aws_vpc", ID: "vpc-sample", Managed: inventory.StatusManaged},
			}}
			got, err := managed.Mark(tc.ctx, tc.run, tc.root, in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error %v, want %v", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "mark resources:") {
				t.Fatalf("error %q does not start with %q", err, "mark resources:")
			}
			if got.Resources[0].Managed != inventory.StatusManaged {
				t.Fatalf("status = %s, want the input status", got.Resources[0].Managed)
			}
			if fake, ok := tc.run.(*fakeRunner); ok && len(fake.calls) != 0 {
				t.Fatalf("calls = %#v, want no command", fake.calls)
			}
		})
	}
}

func TestStandardLibraryOnly(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on PATH")
	}

	pkg := modulePath + "/internal/managed"
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
