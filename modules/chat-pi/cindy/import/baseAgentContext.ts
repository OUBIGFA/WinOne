

/**
 * Main-only send metadata. Symbol keys cannot cross Electron/device-link
 * structured-clone boundaries, so Renderer-controlled send options cannot mint
 * this proof. Main dispatchers attach it only after authenticating the source.
 */
export const MAIN_OWNED_SEND_CONTEXT = Symbol('cindy.main-owned-send-context');


/** Call-local user content before Session replaces images with generated descriptions. */
export const AUTO_REVIEW_SOURCE_CONTENT = Symbol('cindy.auto-review-source-content');


/** Host-restored user authorization for this send; never accepted from wire options. */
export const AUTO_REVIEW_USER_INTENT = Symbol('cindy.auto-review-user-intent');


/** Main-attested continuation: retain initialized live intent, including an explicit empty reset. */
export const AUTO_REVIEW_DELEGATED_CONTINUATION = Symbol('autoReviewDelegatedContinuation');
export {  };
