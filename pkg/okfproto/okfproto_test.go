package okfproto_test

import (
	"testing"
	"time"

	protobundle "github.com/ghchinoy/okf-proto/pkg/bundle"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/ghchinoy/binder/pkg/bundle"
	"github.com/ghchinoy/binder/pkg/graph"
	"github.com/ghchinoy/binder/pkg/okf"
	"github.com/ghchinoy/binder/pkg/okf/native"
	"github.com/ghchinoy/binder/pkg/okfproto"
	"github.com/ghchinoy/binder/pkg/validate"
)

func TestToProtoAndFromProtoRoundTrip(t *testing.T) {
	codec := native.New()
	orig, err := bundle.Load("../../testdata/okf-bundles/acme_retail", codec)
	if err != nil {
		t.Fatalf("bundle.Load() returned unexpected error: %v", err)
	}

	pb, err := okfproto.ToProto(orig, "2026-08-15")
	if err != nil {
		t.Fatalf("okfproto.ToProto() returned unexpected error: %v", err)
	}
	if len(pb.GetConcepts()) != len(orig.Concepts) {
		t.Fatalf("okfproto.ToProto() len(Concepts) = %d, want %d", len(pb.GetConcepts()), len(orig.Concepts))
	}

	// Verify binary wire round-trip and proto.Equal equality.
	wire, err := protobundle.MarshalBinary(pb)
	if err != nil {
		t.Fatalf("protobundle.MarshalBinary() returned unexpected error: %v", err)
	}
	pbFromWire, err := protobundle.UnmarshalBinary(wire)
	if err != nil {
		t.Fatalf("protobundle.UnmarshalBinary() returned unexpected error: %v", err)
	}
	if !proto.Equal(pb, pbFromWire) {
		t.Errorf("protobundle.UnmarshalBinary() did not produce proto.Equal bundle")
	}

	roundTripped, err := okfproto.FromProto(pbFromWire, orig.Root)
	if err != nil {
		t.Fatalf("okfproto.FromProto() returned unexpected error: %v", err)
	}
	if len(roundTripped.Concepts) != len(orig.Concepts) {
		t.Fatalf("okfproto.FromProto() len(Concepts) = %d, want %d", len(roundTripped.Concepts), len(orig.Concepts))
	}

	// Graph projection over the round-tripped bundle must equal the original.
	gOrig := graph.Build(orig, "2026-08-15")
	gRT := graph.Build(roundTripped, "2026-08-15")
	if diff := cmp.Diff(gOrig.Nodes, gRT.Nodes); diff != "" {
		t.Errorf("graph.Build() Nodes after Protobuf round-trip returned unexpected diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(gOrig.Edges, gRT.Edges); diff != "" {
		t.Errorf("graph.Build() Edges after Protobuf round-trip returned unexpected diff (-want +got):\n%s", diff)
	}
}

func TestFromDescriptorSetProducesConformantGraphAndSpannerProjection(t *testing.T) {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("acme/orders/v1/orders.proto"),
				Package: proto.String("acme.orders.v1"),
				Syntax:  proto.String("proto3"),
				EnumType: []*descriptorpb.EnumDescriptorProto{
					{
						Name: proto.String("OrderStatus"),
						Value: []*descriptorpb.EnumValueDescriptorProto{
							{Name: proto.String("ORDER_STATUS_UNSPECIFIED"), Number: proto.Int32(0)},
						},
					},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Order"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("status"),
								Number:   proto.Int32(1),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
								TypeName: proto.String(".acme.orders.v1.OrderStatus"),
							},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("OrderService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("GetOrder"),
								InputType:  proto.String(".acme.orders.v1.Order"),
								OutputType: proto.String(".acme.orders.v1.Order"),
							},
						},
					},
				},
			},
		},
	}

	outDir := t.TempDir()
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	b, pb, err := okfproto.FromDescriptorSet(fds, outDir, now, "binder/0.6.0")
	if err != nil {
		t.Fatalf("okfproto.FromDescriptorSet() returned unexpected error: %v", err)
	}

	// Write bundle to disk and run binder's §11 validator over it.
	if err := okfproto.WriteBundle(pb, outDir); err != nil {
		t.Fatalf("okfproto.WriteBundle() returned unexpected error: %v", err)
	}
	res, err := validate.Bundle(outDir, native.New(), okf.DefaultSpecVersion)
	if err != nil {
		t.Fatalf("validate.Bundle() returned unexpected error: %v", err)
	}
	if !res.Conformant() || len(res.Findings) != 0 {
		t.Errorf("validate.Bundle(%q) findings = %+v, want empty", outDir, res.Findings)
	}

	// Verify that --id-key resource achieves 100% re-rooting stability in Spanner projection.
	proj := graph.Project(b, graph.ProjectOptions{
		Target: graph.TargetSpanner,
		IDKey:  "resource",
		Today:  "2026-08-15",
	})
	if !proj.Identity.ReRootingStable || proj.Identity.PathFallbackCount != 0 {
		t.Errorf("graph.Project() Identity = %+v, want ReRootingStable=true and PathFallbackCount=0", proj.Identity)
	}
	if proj.Counts.Nodes != 3 || proj.Counts.Edges != 2 {
		t.Errorf("graph.Project() Counts = %+v, want 3 nodes and 2 edges", proj.Counts)
	}
}
