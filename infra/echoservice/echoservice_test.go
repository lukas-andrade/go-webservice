package echoservice

import (
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// recordingMocks stores every resource Pulumi would create, so tests can
// inspect the inputs without talking to a cluster.
type recordingMocks struct {
	mu        sync.Mutex
	resources map[string]resource.PropertyMap
}

func (m *recordingMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources[args.TypeToken] = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *recordingMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func deploy(t *testing.T, args Args) *recordingMocks {
	t.Helper()
	mocks := &recordingMocks{resources: map[string]resource.PropertyMap{}}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "echo-service", args)
		return err
	}, pulumi.WithMocks("go-webservice", "test", mocks))
	if err != nil {
		t.Fatalf("pulumi run: %v", err)
	}
	return mocks
}

func TestNewCreatesDeploymentAndService(t *testing.T) {
	mocks := deploy(t, Args{Image: "localhost:5001/echo-service:abc123", Replicas: 3})

	deployment, ok := mocks.resources["kubernetes:apps/v1:Deployment"]
	if !ok {
		t.Fatal("no Deployment was created")
	}
	spec := deployment["spec"].ObjectValue()
	if got := spec["replicas"].NumberValue(); got != 3 {
		t.Errorf("replicas = %v, want 3", got)
	}

	pod := spec["template"].ObjectValue()["spec"].ObjectValue()
	c := pod["containers"].ArrayValue()[0].ObjectValue()
	if got := c["image"].StringValue(); got != "localhost:5001/echo-service:abc123" {
		t.Errorf("image = %q", got)
	}
	for probe, path := range map[string]string{"livenessProbe": "/healthz", "readinessProbe": "/readyz"} {
		get := c[resource.PropertyKey(probe)].ObjectValue()["httpGet"].ObjectValue()
		if get["path"].StringValue() != path || get["port"].StringValue() != "admin" {
			t.Errorf("%s = %v, want %s on the admin port", probe, get, path)
		}
	}
	sc := c["securityContext"].ObjectValue()
	if !sc["readOnlyRootFilesystem"].BoolValue() || sc["allowPrivilegeEscalation"].BoolValue() {
		t.Errorf("container security context too loose: %v", sc)
	}

	service, ok := mocks.resources["kubernetes:core/v1:Service"]
	if !ok {
		t.Fatal("no Service was created")
	}
	port := service["spec"].ObjectValue()["ports"].ArrayValue()[0].ObjectValue()
	if port["port"].NumberValue() != 80 || port["targetPort"].StringValue() != "http" {
		t.Errorf("service port = %v, want 80 -> http", port)
	}

	selector := service["spec"].ObjectValue()["selector"].ObjectValue()
	podLabels := spec["template"].ObjectValue()["metadata"].ObjectValue()["labels"].ObjectValue()
	if !selector.DeepEquals(podLabels) {
		t.Errorf("service selector %v does not match pod labels %v", selector, podLabels)
	}
}

func TestNewDefaultsToOneReplica(t *testing.T) {
	mocks := deploy(t, Args{Image: "echo:dev"})

	spec := mocks.resources["kubernetes:apps/v1:Deployment"]["spec"].ObjectValue()
	if got := spec["replicas"].NumberValue(); got != 1 {
		t.Errorf("replicas = %v, want 1", got)
	}
}

func TestNewRequiresImage(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "echo-service", Args{})
		return err
	}, pulumi.WithMocks("go-webservice", "test", &recordingMocks{resources: map[string]resource.PropertyMap{}}))
	if err == nil {
		t.Fatal("expected an error without an image")
	}
}
