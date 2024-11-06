package a

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("a")

func endless(ctx context.Context) error {
	_, span := tracer.Start(ctx, "a") // want "missing span.End call in the scope"

	sth, err := fetchSomething(ctx)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.String("sth.id", sth.ID))
	return nil
}

func returningSpan(ctx context.Context) (trace.Span, error) {
	_, span := tracer.Start(ctx, "a")
	return span, nil
}

func endful(ctx context.Context) error {
	_, span := tracer.Start(ctx, "a")
	defer span.End()

	sth, err := fetchSomething(ctx)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.String("sth.id", sth.ID))
	return nil
}

func nonDefer(ctx context.Context) error {
	_, span := tracer.Start(ctx, "a")

	sth, err := fetchSomething(ctx)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.String("sth.id", sth.ID))
	span.End() // want "you may want to defer the span.End() call"
	return nil
}

var anonymous = func(ctx context.Context) error {
	_, span := tracer.Start(ctx, "a") // want "missing span.End() call in the scope"

	sth, err := fetchSomething(ctx)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.String("sth.id", sth.ID))
	return nil
}

var anonymousReturningSpan = func(ctx context.Context) (trace.Span, error) {
	_, span := tracer.Start(ctx, "a")

	return span, nil
}

type something struct {
	ID string
}

func fetchSomething(ctx context.Context) (something, error) {
	time.Sleep(100 * time.Millisecond)

	return something{
		ID: "hoge",
	}, nil
}
