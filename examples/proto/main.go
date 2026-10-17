// Command proto-example showcases combining Protobuf/Buf API schemas with
// institutional Markdown knowledge (Architecture Decision Records and SQL
// Metrics) in a single conformant Open Knowledge Format (OKF) v0.2 bundle using
// pkg/okfproto and pkg/binder.
//
// It demonstrates four capabilities that neither a Protobuf compiler nor a
// standalone Markdown wiki can perform on its own:
//
//  1. Schema Ingestion (okfproto.FromDescriptorSet): projects a compiled
//     google.protobuf.FileDescriptorSet (OrderService, Order, LineItem,
//     OrderStatus, and a deprecated LegacyOrderV0) into linked OKF concepts.
//  2. Hybrid Knowledge Graph Assembly: adds a human-reviewed Architecture
//     Decision Record (decisions/adr-012-order-cancellation.md) and a Finance
//     Metric (metrics/recognized-revenue.md) that link directly to the Protobuf
//     concepts, then regenerates the bundle navigation via Service.Index.
//  3. Topology & Governance Review (Service.Review): classifies OrderService and
//     the ADR/Metric as graph entrypoints, separates human-reviewed prose from
//     unverified generated schemas, and catches LegacyOrderV0 as a deprecated
//     true orphan (0 inbound and 0 outbound links).
//  4. Cross-Domain Blast-Radius & Lineage Queries (Service.GraphQuery):
//     - Inbound neighbors of enums/acme/orders/v1/OrderStatus (answers: "if we
//     change OrderStatus, which Protobuf messages, ADRs, and SQL metrics are
//     affected?").
//     - Shortest path from metrics/recognized-revenue to
//     services/acme/orders/v1/OrderService across the prose-to-proto boundary.
//
// Usage:
//
//	go run ./examples/proto
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	protobundle "github.com/ghchinoy/okf-proto/pkg/bundle"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/ghchinoy/binder/pkg/binder"
	"github.com/ghchinoy/binder/pkg/binder/render"
	"github.com/ghchinoy/binder/pkg/bundle"
	"github.com/ghchinoy/binder/pkg/okf/native"
	"github.com/ghchinoy/binder/pkg/okfproto"
)

func main() {
	version := os.Getenv("BINDER_VERSION")
	if version == "" {
		version = "example"
	}

	// Pin the clock (honoring SOURCE_DATE_EPOCH when set) for deterministic output.
	fixedDefault := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	now, err := binder.ResolveNow(os.Getenv("SOURCE_DATE_EPOCH"), fixedDefault)
	if err != nil {
		fail(err)
	}
	today := now.Format("2006-01-02")

	workDir, err := os.MkdirTemp("", "binder-proto-example-")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(workDir)

	bundleDir := filepath.Join(workDir, "acme-orders-kb")

	// 1. Ingest the Protobuf FileDescriptorSet into an on-disk OKF v0.2 bundle.
	fmt.Fprintln(os.Stderr, "==> 1. Ingesting Protobuf FileDescriptorSet (acme.orders.v1)")
	_, pbBundle, err := okfproto.FromDescriptorSet(sampleOrdersFDS(), bundleDir, now, "okf-proto/example")
	if err != nil {
		fail(err)
	}
	if err := okfproto.WriteBundle(pbBundle, bundleDir); err != nil {
		fail(err)
	}

	// 2. Layer human-reviewed institutional knowledge (an ADR and a Finance Metric)
	// that link directly to the generated Protobuf concepts.
	fmt.Fprintln(os.Stderr, "==> 2. Adding human-reviewed ADR and Finance Metric linking to Protobuf concepts")
	if err := writeProseConcepts(bundleDir); err != nil {
		fail(err)
	}

	codec := native.New()
	svc := binder.New(codec)
	ctx := context.Background()

	// Regenerate per-directory index.md navigation (with type-grouped catalog and backlinks).
	if _, err := svc.Index(ctx, binder.IndexRequest{
		Root:             bundleDir,
		GroupByType:      true,
		IncludeBacklinks: true,
		IncludeGraph:     true,
	}); err != nil {
		fail(err)
	}

	// 3. Validate and Review the hybrid bundle.
	fmt.Fprintln(os.Stderr, "==> 3. Validating and reviewing hybrid bundle (prose + Protobuf)")
	valRes, err := svc.Validate(ctx, binder.ValidateRequest{
		Bundle:  bundleDir,
		Version: version,
	})
	if err != nil {
		fail(err)
	}
	fmt.Print(render.Validate(valRes))

	revRes, err := svc.Review(ctx, binder.ReviewRequest{
		Bundle:  bundleDir,
		Now:     now,
		Today:   today,
		Version: version,
	})
	if err != nil {
		fail(err)
	}
	fmt.Print(render.Review(revRes))

	// 4. Cross-domain blast-radius query: "What depends on OrderStatus?"
	fmt.Fprintln(os.Stderr, "==> 4. Blast-radius query: inbound neighbors of enums/acme/orders/v1/OrderStatus")
	nbRes, err := svc.GraphQuery(ctx, binder.GraphQueryRequest{
		Bundle:    bundleDir,
		Op:        binder.OpNeighbors,
		ID:        "enums/acme/orders/v1/OrderStatus",
		Direction: "in",
		Now:       now,
		Today:     today,
		Version:   version,
	})
	if err != nil {
		fail(err)
	}
	if err := nbRes.EncodeJSON(os.Stdout); err != nil {
		fail(err)
	}

	// 5. Cross-domain lineage path: Metric -> Protobuf Message -> Protobuf Service.
	fmt.Fprintln(os.Stderr, "==> 5. Lineage path: metrics/recognized-revenue <-> services/acme/orders/v1/OrderService")
	pathRes, err := svc.GraphQuery(ctx, binder.GraphQueryRequest{
		Bundle:    bundleDir,
		Op:        binder.OpPath,
		From:      "metrics/recognized-revenue",
		To:        "services/acme/orders/v1/OrderService",
		Direction: "both",
		MaxDepth:  5,
		Now:       now,
		Today:     today,
		Version:   version,
	})
	if err != nil {
		fail(err)
	}
	if err := pathRes.EncodeJSON(os.Stdout); err != nil {
		fail(err)
	}

	// 6. Export the entire hybrid bundle to deterministic Protobuf wire bytes (.binpb).
	loaded, err := bundle.Load(bundleDir, codec)
	if err != nil {
		fail(err)
	}
	wireMsg, err := okfproto.ToProto(loaded, today)
	if err != nil {
		fail(err)
	}
	wireBytes, err := protobundle.MarshalBinary(wireMsg)
	if err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "==> 6. Exported hybrid OKF bundle (%d concepts) to %d bytes of deterministic Protobuf wire format\n",
		len(wireMsg.GetConcepts()), len(wireBytes))
}

func writeProseConcepts(bundleDir string) error {
	adr := "---\n" +
		"type: Decision\n" +
		"title: \"ADR-012: Order Cancellation Semantics\"\n" +
		"description: Defines when a customer order may transition to ORDER_STATUS_CANCELLED.\n" +
		"status: stable\n" +
		"generated: { by: human:arch-board, at: 2026-07-01T10:00:00Z }\n" +
		"verified:\n" +
		"  - { by: human:staff-eng@acme, at: 2026-07-02T15:00:00Z }\n" +
		"---\n\n" +
		"# ADR-012: Order Cancellation Semantics\n\n" +
		"Governs cancellation transitions on [OrderService](/services/acme/orders/v1/OrderService.md) " +
		"and the [OrderStatus](/enums/acme/orders/v1/OrderStatus.md) lifecycle enum.\n"

	metric := "---\n" +
		"type: Metric\n" +
		"title: Recognized Revenue\n" +
		"description: Net revenue from fulfilled customer orders.\n" +
		"status: stable\n" +
		"stale_after: 2026-12-31T00:00:00Z\n" +
		"generated: { by: human:finance-fpa, at: 2026-07-10T09:00:00Z }\n" +
		"verified:\n" +
		"  - { by: human:controller@acme, at: 2026-07-11T11:00:00Z }\n" +
		"---\n\n" +
		"# Recognized Revenue\n\n" +
		"Sums [LineItem](/messages/acme/orders/v1/LineItem.md) unit prices on every " +
		"[Order](/messages/acme/orders/v1/Order.md) whose [OrderStatus](/enums/acme/orders/v1/OrderStatus.md) " +
		"has reached `ORDER_STATUS_SHIPPED`.\n"

	for rel, content := range map[string]string{
		"decisions/adr-012-order-cancellation.md": adr,
		"metrics/recognized-revenue.md":           metric,
	} {
		target := filepath.Join(bundleDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sampleOrdersFDS() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
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
							{Name: proto.String("ORDER_STATUS_PENDING"), Number: proto.Int32(1)},
							{Name: proto.String("ORDER_STATUS_SHIPPED"), Number: proto.Int32(2)},
							{Name: proto.String("ORDER_STATUS_CANCELLED"), Number: proto.Int32(3)},
						},
					},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("LineItem"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: proto.String("sku"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
							{Name: proto.String("quantity"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()},
							{Name: proto.String("unit_price_cents"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum()},
						},
					},
					{
						Name: proto.String("Order"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: proto.String("order_id"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
							{
								Name:     proto.String("status"),
								Number:   proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
								TypeName: proto.String(".acme.orders.v1.OrderStatus"),
							},
							{
								Name:     proto.String("items"),
								Number:   proto.Int32(3),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: proto.String(".acme.orders.v1.LineItem"),
							},
						},
					},
					{
						Name: proto.String("GetOrderRequest"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: proto.String("order_id"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
						},
					},
					{
						Name:    proto.String("LegacyOrderV0"),
						Options: &descriptorpb.MessageOptions{Deprecated: proto.Bool(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{Name: proto.String("legacy_id"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("OrderService"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("GetOrder"),
								InputType:  proto.String(".acme.orders.v1.GetOrderRequest"),
								OutputType: proto.String(".acme.orders.v1.Order"),
							},
						},
					},
				},
			},
		},
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "proto-example:", err)
	os.Exit(1)
}
