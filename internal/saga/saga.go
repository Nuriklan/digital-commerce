package saga

import (
	"context"
	"fmt"
)

type Step interface {
	Name() string
	Execute(ctx context.Context) error
	Compensate(ctx context.Context) error
}

type Orchestrator struct {
	steps []Step
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{}
}

func (o *Orchestrator) AddStep(step Step) *Orchestrator {
	o.steps = append(o.steps, step)
	return o
}

func (o *Orchestrator) Execute(ctx context.Context) error {
	var executedSteps []Step

	for _, step := range o.steps {
		if ctx.Err() != nil {
			_ = o.compensate(context.Background(), executedSteps)
			return ctx.Err()
		}

		if err := step.Execute(ctx); err != nil {
			compErr := o.compensate(context.Background(), executedSteps)
			if compErr != nil {
				return fmt.Errorf("step %q failed: %w; compensation errors: %v", step.Name(), err, compErr)
			}
			return fmt.Errorf("step %q failed: %w (compensated successfully)", step.Name(), err)
		}

		executedSteps = append(executedSteps, step)
	}

	return nil
}

func (o *Orchestrator) compensate(ctx context.Context, steps []Step) error {
	var errs []error
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if err := step.Compensate(ctx); err != nil {
			errs = append(errs, fmt.Errorf("step %q compensation failed: %w", step.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}
