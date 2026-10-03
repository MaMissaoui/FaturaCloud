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

// RunDue executes and clears every task scheduled for exactly this date,
// including any a task schedules for the same date while it runs (a bill on
// the day of its receipt): it keeps draining until the date has nothing left.
// Before it did, a same-day follow-up was appended to an already-taken slice
// and silently never ran — 18 of 125 receipts in a 27-month retail run were
// never billed, leaving their value stuck on Goods Received Not Invoiced.
// Run calls it again at the end of each day for tasks the day's generators
// schedule for that same day.
func (sc *Scheduler) RunDue(date time.Time, onErr func(error)) {
	key := date.Format("2006-01-02")
	for {
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
}
