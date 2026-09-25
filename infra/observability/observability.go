package observability

import (
	"fmt"
	"os"
	"path/filepath"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"sigs.k8s.io/yaml"
)

const (
	namespace      = "observability"
	communityRepo  = "https://grafana-community.github.io/helm-charts"
	otelChartsRepo = "https://open-telemetry.github.io/opentelemetry-helm-charts"
	mimirImage     = "grafana/mimir:3.2.1"
)

type chart struct {
	name, repo, chart, version string
}

var charts = []chart{
	{"loki", communityRepo, "loki", "18.13.5"},
	{"tempo", communityRepo, "tempo", "3.0.0"},
	{"grafana", communityRepo, "grafana", "13.2.5"},
	{"otel-collector", otelChartsRepo, "opentelemetry-collector", "0.173.1"},
}

type Args struct {
	// ConfigDir is the repo's observability/ directory, whose files are
	// shared with docker compose.
	ConfigDir string
}

// Stack is the whole LGTM setup as one component: Loki, Tempo, Grafana and
// the OTel Collector from Helm, plus a single-process Mimir.
type Stack struct {
	pulumi.ResourceState

	// OTLPEndpoint is where applications send traces.
	OTLPEndpoint string
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Stack, error) {
	stack := &Stack{OTLPEndpoint: fmt.Sprintf("http://otel-collector.%s:4318", namespace)}
	if err := ctx.RegisterComponentResource("go-webservice:index:Observability", name, stack, opts...); err != nil {
		return nil, err
	}

	ns, err := corev1.NewNamespace(ctx, namespace, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(namespace)},
	}, pulumi.Parent(stack))
	if err != nil {
		return nil, err
	}
	inNamespace := []pulumi.ResourceOption{pulumi.Parent(stack), pulumi.DependsOn([]pulumi.Resource{ns})}

	if err := newMimir(ctx, filepath.Join(args.ConfigDir, "mimir.yaml"), inNamespace); err != nil {
		return nil, err
	}

	datasources, err := readYAML(filepath.Join(args.ConfigDir, "grafana-datasources.yaml"))
	if err != nil {
		return nil, err
	}
	extraValues := map[string]pulumi.Map{
		"grafana": {"datasources": pulumi.Map{"datasources.yaml": pulumi.ToMap(datasources)}},
	}

	for _, c := range charts {
		_, err := helmv3.NewRelease(ctx, c.name, &helmv3.ReleaseArgs{
			Name:           pulumi.String(c.name),
			Namespace:      pulumi.String(namespace),
			Chart:          pulumi.String(c.chart),
			Version:        pulumi.String(c.version),
			RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(c.repo)},
			ValueYamlFiles: pulumi.AssetOrArchiveArray{
				pulumi.NewFileAsset(filepath.Join(args.ConfigDir, "helm", c.name+".yaml")),
			},
			Values:  extraValues[c.name],
			Timeout: pulumi.Int(300),
		}, inNamespace...)
		if err != nil {
			return nil, err
		}
	}

	return stack, ctx.RegisterResourceOutputs(stack, pulumi.Map{
		"otlpEndpoint": pulumi.String(stack.OTLPEndpoint),
	})
}

// Mimir's only chart is the microservices one, which is ten-plus pods. For a
// local cluster the single-process mode with the compose config is enough.
func newMimir(ctx *pulumi.Context, configPath string, opts []pulumi.ResourceOption) error {
	config, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read mimir config: %w", err)
	}
	labels := pulumi.StringMap{"app.kubernetes.io/name": pulumi.String("mimir")}
	meta := &metav1.ObjectMetaArgs{Name: pulumi.String("mimir"), Namespace: pulumi.String(namespace), Labels: labels}

	cm, err := corev1.NewConfigMap(ctx, "mimir", &corev1.ConfigMapArgs{
		Metadata: meta,
		Data:     pulumi.StringMap{"mimir.yaml": pulumi.String(config)},
	}, opts...)
	if err != nil {
		return err
	}

	_, err = appsv1.NewDeployment(ctx, "mimir", &appsv1.DeploymentArgs{
		Metadata: meta,
		Spec: &appsv1.DeploymentSpecArgs{
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{&corev1.ContainerArgs{
						Name:  pulumi.String("mimir"),
						Image: pulumi.String(mimirImage),
						Args:  pulumi.StringArray{pulumi.String("-target=all"), pulumi.String("-config.file=/etc/mimir/mimir.yaml")},
						Ports: corev1.ContainerPortArray{&corev1.ContainerPortArgs{Name: pulumi.String("http"), ContainerPort: pulumi.Int(9009)}},
						ReadinessProbe: &corev1.ProbeArgs{
							HttpGet: &corev1.HTTPGetActionArgs{Path: pulumi.String("/ready"), Port: pulumi.String("http")},
						},
						VolumeMounts: corev1.VolumeMountArray{
							&corev1.VolumeMountArgs{Name: pulumi.String("config"), MountPath: pulumi.String("/etc/mimir")},
							&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/tmp/mimir")},
						},
					}},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{Name: pulumi.String("config"), ConfigMap: &corev1.ConfigMapVolumeSourceArgs{Name: cm.Metadata.Name()}},
						&corev1.VolumeArgs{Name: pulumi.String("data"), EmptyDir: &corev1.EmptyDirVolumeSourceArgs{}},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return err
	}

	_, err = corev1.NewService(ctx, "mimir", &corev1.ServiceArgs{
		Metadata: meta,
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports: corev1.ServicePortArray{&corev1.ServicePortArgs{
				Name: pulumi.String("http"), Port: pulumi.Int(9009), TargetPort: pulumi.String("http"),
			}},
		},
	}, opts...)
	return err
}

func readYAML(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var out map[string]any
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}
