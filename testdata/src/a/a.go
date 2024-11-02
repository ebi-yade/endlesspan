package a

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("a")

func endless(ctx context.Context) error {
	_, span := tracer.Start(ctx, "a") // want "missing span.End() in the function scope"

	sth, err := fetchSomething(ctx)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.String("sth.id", sth.ID))
	return nil
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

type something struct {
	ID string
}

func fetchSomething(ctx context.Context) (something, error) {
	time.Sleep(100 * time.Millisecond)

	return something{
		ID: "hoge",
	}, nil
}
