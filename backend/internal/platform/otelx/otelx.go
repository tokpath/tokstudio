// Package otelx 初始化 OpenTelemetry。未配置 OTLP 时，开发环境使用 stdout exporter。
package otelx

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup 安装全局 TracerProvider，返回 shutdown。
func Setup(ctx context.Context, serviceName, otlpEndpoint string) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	var exp sdktrace.SpanExporter
	if otlpEndpoint != "" {
		exp, err = otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(otlpEndpoint), otlptracehttp.WithInsecure())
		if err != nil {
			return nil, fmt.Errorf("otlp exporter: %w", err)
		}
	} else {
		exp, err = stdouttrace.New(stdouttrace.WithWriter(os.Stdout), stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("stdout exporter: %w", err)
		}
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(200*time.Millisecond)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return provider.Shutdown, nil
}
