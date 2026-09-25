package observability

import (
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type recordingMocks struct {
	mu        sync.Mutex
	resources map[string]resource.PropertyMap
}

func (m *recordingMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources[args.TypeToken+"::"+args.Name] = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *recordingMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func TestNewInstallsTheWholeStack(t *testing.T) {
	mocks := &recordingMocks{resources: map[string]resource.PropertyMap{}}
	var stack *Stack
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		var err error
		stack, err = New(ctx, "observability", Args{ConfigDir: "../../observability"})
		return err
	}, pulumi.WithMocks("go-webservice", "test", mocks))
	if err != nil {
		t.Fatalf("pulumi run: %v", err)
	}

	for _, c := range charts {
		release, ok := mocks.resources["kubernetes:helm.sh/v3:Release::"+c.name]
		if !ok {
			t.Errorf("no Helm release for %s", c.name)
			continue
		}
		if got := release["version"].StringValue(); got != c.version {
			t.Errorf("%s chart version = %s, want %s", c.name, got, c.version)
		}
		if got := release["namespace"].StringValue(); got != namespace {
			t.Errorf("%s namespace = %s", c.name, got)
		}
	}

	for _, kind := range []string{"core/v1:ConfigMap", "apps/v1:Deployment", "core/v1:Service"} {
		if _, ok := mocks.resources["kubernetes:"+kind+"::mimir"]; !ok {
			t.Errorf("no Mimir %s", kind)
		}
	}
	cm := mocks.resources["kubernetes:core/v1:ConfigMap::mimir"]
	if !strings.Contains(cm["data"].ObjectValue()["mimir.yaml"].StringValue(), "blocks_storage") {
		t.Error("Mimir ConfigMap should carry observability/mimir.yaml")
	}

	dashboard, ok := mocks.resources["kubernetes:core/v1:ConfigMap::echo-service-dashboard"]
	if !ok {
		t.Fatal("no Grafana dashboard ConfigMap")
	}
	dashboardJSON := dashboard["data"].ObjectValue()["echo-service-overview.json"].StringValue()
	for _, expected := range []string{"Echo Service Overview", "HTTP 5xx error rate", "p95 request latency", "Logs"} {
		if !strings.Contains(dashboardJSON, expected) {
			t.Errorf("dashboard is missing %q", expected)
		}
	}

	grafana := mocks.resources["kubernetes:helm.sh/v3:Release::grafana"]
	datasources := grafana["values"].ObjectValue()["datasources"].ObjectValue()["datasources.yaml"].ObjectValue()
	if n := len(datasources["datasources"].ArrayValue()); n != 3 {
		t.Errorf("grafana got %d datasources, want Mimir, Tempo and Loki", n)
	}

	if stack.OTLPEndpoint != "http://otel-collector.observability:4318" {
		t.Errorf("OTLPEndpoint = %s", stack.OTLPEndpoint)
	}
}

func TestNewFailsWithoutConfigFiles(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "observability", Args{ConfigDir: t.TempDir()})
		return err
	}, pulumi.WithMocks("go-webservice", "test", &recordingMocks{resources: map[string]resource.PropertyMap{}}))
	if err == nil {
		t.Fatal("expected an error when the shared config files are missing")
	}
}
