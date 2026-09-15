package worker

import (
	"context"
	"fmt"
	"sync"
)

type Job struct {
	Name    string
	Payload any
}

type HandlerFunc func(ctx context.Context, job Job) error

type Pool struct {
	workers int
	jobs    chan Job
	handler HandlerFunc
	wg      sync.WaitGroup
}

func NewPool(workers, bufferSize int, handler HandlerFunc) *Pool {
	return &Pool{
		workers: workers,
		jobs:    make(chan Job, bufferSize),
		handler: handler,
	}
}

func (p *Pool) Start(ctx context.Context) {
	for i := range p.workers {
		p.wg.Add(1)
		go p.runWorker(ctx, i)
	}
}

func (p *Pool) Submit(ctx context.Context, job Job) bool {
	select {
	case p.jobs <- job:
		return true
	case <-ctx.Done():
		return false
	}
}

func (p *Pool) Wait() {
	close(p.jobs)
	p.wg.Wait()
}

func (p *Pool) runWorker(ctx context.Context, id int) {
	defer p.wg.Done()

	fmt.Printf("[worker %d] started\n", id)

	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				fmt.Printf("[worker %d] shutting down\n", id)
				return
			}
			if err := p.handler(ctx, job); err != nil {
				fmt.Printf("[worker %d] job %q failed: %v\n", id, job.Name, err)
			}
		case <-ctx.Done():
			fmt.Printf("[worker %d] context cancelled, draining...\n", id)
			for job := range p.jobs {
				if err := p.handler(ctx, job); err != nil {
					fmt.Printf("[worker %d] job %q failed: %v\n", id, job.Name, err)
				}
			}
			return
		}
	}
}
