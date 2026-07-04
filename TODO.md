# TODO

- **Replace the placeholder Sentry DSN** in `src/utils/sentry.ts` with your own
  Sentry project's DSN. It's currently a dummy value
  (`https://dummy-replace-me@o000000.ingest.us.sentry.io/0000000`), which
  makes crash reporting and the feedback modal (`src/components/feedback-modal.tsx`)
  no-ops until replaced. The dummy value stands in for a previously hardcoded
  third-party DSN inherited from the original Fatura desktop app, which was
  silently sending this deployment's crash reports and user feedback
  submissions to an account we don't control.
