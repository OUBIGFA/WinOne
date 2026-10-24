import {defineConfig} from 'vitest/config';
// Run the original Chitchatter protocol tests without a DOM or UI mocks.
export default defineConfig({test:{environment:'node',include:['src/vendor/chitchatter/core/**/*.test.ts'],maxWorkers:2}});
