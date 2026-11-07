// Cindy's maker logger writes to its application log; the plugin host reports
// through its own diagnostic event, so routine messages stay out of stdout.
const quiet = () => {};
export const desktopMakerLogger = {debug: quiet, info: quiet, warn: quiet, error: quiet, child() { return desktopMakerLogger; }};
