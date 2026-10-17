import { z } from 'zod';

import type { ClientInit } from '@sveltejs/kit/hooks';

// zod compiles fast object parsers with the Function constructor, after it
// probes whether the page allows that. The Content-Security-Policy forbids
// both, and the probe alone reports a violation (ADR-032). Without its JIT,
// zod never probes.
export const init: ClientInit = () => {
	z.config({ jitless: true });
};
