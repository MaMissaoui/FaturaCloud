import * as Sentry from "@sentry/react";
import { GetVersion } from "src/api";

// Initialize Sentry
export const initSentry = async () => {
  // DSN comes from the VITE_SENTRY_DSN build-time env var, not a literal here —
  // this file ships in the public GHCR image, and a hardcoded DSN would mean
  // every downstream deployer's crash reports land in this project's own
  // Sentry account. Set VITE_SENTRY_DSN as a Docker build-arg for deployments
  // that want error tracking; the published image is built without it.
  const dsn = import.meta.env.VITE_SENTRY_DSN;

  // Check if Sentry should be enabled
  const isEnabled =
    Boolean(dsn) &&
    (import.meta.env.VITE_SENTRY_ENABLED === "true" ||
      (import.meta.env.VITE_SENTRY_ENABLED === undefined && import.meta.env.PROD));

  // Get the app version
  let appVersion = "unknown";
  try {
    appVersion = await GetVersion();
  } catch (error) {
    console.warn("Failed to get app version:", error);
  }

  Sentry.init({
    dsn,
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
