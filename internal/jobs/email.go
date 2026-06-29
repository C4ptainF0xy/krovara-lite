package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"
)

type EmailSender interface {
	Send(ctx context.Context, to, subject, html string) error
}

type EmailArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

func (EmailArgs) Kind() string { return "krovara.email" }

func (EmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5}
}

type EmailWorker struct {
	river.WorkerDefaults[EmailArgs]
	Sender EmailSender
}

var ErrNoSender = errors.New("jobs: email worker has no Sender configured")

func (w *EmailWorker) Work(ctx context.Context, job *river.Job[EmailArgs]) error {
	if w.Sender == nil {
		return ErrNoSender
	}
	if job.Args.To == "" {
		return fmt.Errorf("jobs: empty 'to' on email job %d", job.ID)
	}
	return w.Sender.Send(ctx, job.Args.To, job.Args.Subject, job.Args.HTML)
}
