package echoservice

import (
	"errors"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	httpPort  = 8080
	adminPort = 9090
)

type Args struct {
	Image    string
	Replicas int
}

// EchoService groups the Deployment and Service into one Pulumi component,
// so callers deal with a single resource instead of wiring Kubernetes
// objects by hand.
type EchoService struct {
	pulumi.ResourceState

	DeploymentName pulumi.StringOutput
	ServiceName    pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*EchoService, error) {
	if args.Image == "" {
		return nil, errors.New("echoservice: image is required")
	}
	if args.Replicas <= 0 {
		args.Replicas = 1
	}

	svc := &EchoService{}
	if err := ctx.RegisterComponentResource("go-webservice:index:EchoService", name, svc, opts...); err != nil {
		return nil, err
	}
	child := pulumi.Parent(svc)
	labels := pulumi.StringMap{"app.kubernetes.io/name": pulumi.String(name)}
	// Fixed names instead of Pulumi's random suffix, so kubectl commands in
	// the docs keep working.
	meta := &metav1.ObjectMetaArgs{Name: pulumi.String(name), Labels: labels}

	deployment, err := appsv1.NewDeployment(ctx, name, &appsv1.DeploymentArgs{
		Metadata: meta,
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(args.Replicas),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{container(name, args.Image)},
					SecurityContext: &corev1.PodSecurityContextArgs{
						RunAsNonRoot:   pulumi.Bool(true),
						SeccompProfile: &corev1.SeccompProfileArgs{Type: pulumi.String("RuntimeDefault")},
					},
				},
			},
		},
	}, child, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "3m", Update: "3m"}))
	if err != nil {
		return nil, err
	}

	service, err := corev1.NewService(ctx, name, &corev1.ServiceArgs{
		Metadata: meta,
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports: corev1.ServicePortArray{&corev1.ServicePortArgs{
				Name:       pulumi.String("http"),
				Port:       pulumi.Int(80),
				TargetPort: pulumi.String("http"),
			}},
		},
	}, child)
	if err != nil {
		return nil, err
	}

	svc.DeploymentName = deployment.Metadata.Name().Elem()
	svc.ServiceName = service.Metadata.Name().Elem()
	return svc, ctx.RegisterResourceOutputs(svc, pulumi.Map{
		"deploymentName": svc.DeploymentName,
		"serviceName":    svc.ServiceName,
	})
}

// Probes hit the admin port, which is where the service exposes health.
func container(name, image string) *corev1.ContainerArgs {
	probe := func(path string) *corev1.ProbeArgs {
		return &corev1.ProbeArgs{
			HttpGet: &corev1.HTTPGetActionArgs{Path: pulumi.String(path), Port: pulumi.String("admin")},
		}
	}
	return &corev1.ContainerArgs{
		Name:  pulumi.String(name),
		Image: pulumi.String(image),
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{Name: pulumi.String("http"), ContainerPort: pulumi.Int(httpPort)},
			&corev1.ContainerPortArgs{Name: pulumi.String("admin"), ContainerPort: pulumi.Int(adminPort)},
		},
		LivenessProbe:  probe("/healthz"),
		ReadinessProbe: probe("/readyz"),
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{"cpu": pulumi.String("50m"), "memory": pulumi.String("32Mi")},
			Limits:   pulumi.StringMap{"memory": pulumi.String("128Mi")},
		},
		SecurityContext: &corev1.SecurityContextArgs{
			AllowPrivilegeEscalation: pulumi.Bool(false),
			ReadOnlyRootFilesystem:   pulumi.Bool(true),
			Capabilities:             &corev1.CapabilitiesArgs{Drop: pulumi.StringArray{pulumi.String("ALL")}},
		},
	}
}
