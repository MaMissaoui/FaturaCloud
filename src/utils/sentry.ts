import * as Sentry from "@sentry/react";
import { GetVersion } from "src/api";

// Initialize Sentry
export const initSentry = async () => {
  // Check if Sentry should be enabled
  const isEnabled =
    import.meta.env.VITE_SENTRY_ENABLED === "true" ||
    (import.meta.env.VITE_SENTRY_ENABLED === undefined && import.meta.env.PROD);

  // Get the app version
  let appVersion = "unknown";
  try {
    appVersion = await GetVersion();
  } catch (error) {
    console.warn("Failed to get app version:", error);
  }

  Sentry.init({
    // TODO: replace with your own Sentry project's DSN — this is a dummy
    // placeholder. The previous value was a hardcoded third-party DSN
    // inherited from the original Fatura desktop app, silently sending this
    // deployment's crash reports and user feedback to an account we don't
    // control. Create a project at sentry.io and paste its DSN here.
    dsn: "https://dummy-replace-me@o000000.ingest.us.sentry.io/0000000",
    environment: import.meta.env.MODE,
    release: appVersion,
    enabled: isEnabled,
    // Removed feedbackIntegration since we're using a custom modal
    beforeSend(event) {
      // Show crash report dialog for exceptions when enabled
      if (event.exception && event.event_id && isEnabled) {
        Sentry.showReportDialog({ eventId: event.event_id });
      }
      return event;
    },
  });
};

// Export Sentry for use in components
export { Sentry };
