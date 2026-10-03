// Package fsm provides a minimal deterministic finite state machine.
package fsm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type State string

// ErrNoTransition is returned when a handler produces a state that has no register handler and is not a terminal state
var ErrNoTransition = errors.New("fsm: no transition registered for state")

type Handler[T any] func(ctx context.Context, data *T) (State, error)

type Transition struct {
	From     State
	To       State
	Duration time.Duration
	Err      error
}

// Machine is a deterministic FSM.
type Machine[T any] struct {
	initial   State
	terminals map[State]bool
	handlers  map[State]Handler[T]
	fallbacks map[State]State
	maxHops   int
}

type Option[T any] func(*Machine[T])

func WithMaxHops[T any](n int) Option[T] {
	return func(m *Machine[T]) {
		m.maxHops = n
	}
}

func New[T any](initial State, opts ...Option[T]) *Machine[T] {
	m := &Machine[T]{
		initial:   initial,
		terminals: make(map[State]bool),
		handlers:  make(map[State]Handler[T]),
		fallbacks: make(map[State]State),
		maxHops:   32,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Machine[T]) Handle(state State, h Handler[T]) *Machine[T] {
	m.handlers[state] = h
	return m
}

func (m *Machine[T]) Terminal(states ...State) *Machine[T] {
	for _, s := range states {
		m.terminals[s] = true
	}
	return m
}

func (m *Machine[T]) FallbackTo(state State, target State) *Machine[T] {
	m.fallbacks[state] = target
	return m
}

func (m *Machine[T]) Run(ctx context.Context, data *T) ([]Transition, error) {
	trace := make([]Transition, 0,  8)
	current := m.initial

	for hops := 0; hops < m.maxHops; hops++ {
		if ctx.Err() != nil {
			return trace, fmt.Errorf("fsm: context canceled at %q: %w", current, ctx.Err())
		}

		handler, ok := m.handlers[current]

		if !ok {
			return trace, fmt.Errorf("%w: %q", ErrNoTransition, current)
		}

		start := time.Now()
		next, err := handler(ctx, data)
		duration := time.Since(start)

		t := Transition{
			From:     current,
			To:       next,
			Duration: duration,
			Err:      err,
		}

		trace = append(trace, t)

		if err != nil {
			fallback, ok := m.fallbacks[current]

			if !ok {
				trace = append(trace, t)
			}

			t.To = fallback
			next = fallback
		}

		if m.terminals[next] {
			return trace, nil
		}

		current = next
	}

	return trace, fmt.Errorf("fsm: exceeded %d hops starting at %q", m.maxHops, m.initial)
}
