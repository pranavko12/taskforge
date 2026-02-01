package api

import (
	"context"
	"errors"
)

type DependencyChecker interface {
	Check(ctx context.Context) error
}

type DefaultDependencyChecker struct {
	store Store
	queue Queue
}

func NewDependencyChecker(store Store, queue Queue) *DefaultDependencyChecker {
	return &DefaultDependencyChecker{store: store, queue: queue}
}

func (d *DefaultDependencyChecker) Check(ctx context.Context) error {
	if d.store == nil {
		return errors.New("postgres not ready")
	}
	if err := d.store.Ping(ctx); err != nil {
		return errors.New("postgres not ready")
	}
	if d.queue == nil {
		return errors.New("redis not ready")
	}
	if err := d.queue.Ping(ctx); err != nil {
		return errors.New("redis not ready")
	}
	return nil
}
