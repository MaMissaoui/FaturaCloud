package main

import "time"

// Scheduler is a day-keyed queue of follow-up steps — the mechanism every
// multi-stage workflow in this tool (an invoice's eventual payment, a PO's
// eventual receipt/bill/payment) uses to say "come back and do this later."
// Run's day-stepper calls RunDue once per simulated day; nothing here is
// concurrent, so a plain map is enough — no locking, no priority queue.
type Scheduler struct {
	byDate map[string][]func() error
}

func NewScheduler() *Scheduler {
	return &Scheduler{byDate: map[string][]func() error{}}
}

// Schedule queues fn to run the first time RunDue reaches on or after date.
// If date is a weekend, it's snapped forward via businessDaysLater by the
// caller (schedulers here always pass an already-snapped date — see
// businessDaysLater) so a task never silently waits an extra day for RunDue
// to catch it on a day the day-stepper only half-processes.
func (sc *Scheduler) Schedule(date time.Time, fn func() error) {
	key := date.Format("2006-01-02")
	sc.byDate[key] = append(sc.byDate[key], fn)
}

// RunDue executes and clears every task scheduled for exactly this date.
// Because Schedule always receives a business day, a task scheduled for a
// date the main loop actually visits is guaranteed to run — there's no
// carry-forward logic to reconcile.
func (sc *Scheduler) RunDue(date time.Time, onErr func(error)) {
	key := date.Format("2006-01-02")
	tasks := sc.byDate[key]
	if len(tasks) == 0 {
		return
	}
	delete(sc.byDate, key)
	for _, fn := range tasks {
		if err := fn(); err != nil {
			onErr(err)
		}
	}
}
