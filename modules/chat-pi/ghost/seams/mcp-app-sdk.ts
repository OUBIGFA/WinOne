// Bun embeds the exact official browser bundle as text in the Host worker.
// Do not rebuild a replacement App SDK or persist these bytes in App cards.
import source from '@modelcontextprotocol/ext-apps/app-with-deps' with {type: 'text'};
export default source;
