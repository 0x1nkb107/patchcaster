// A change names the version it was made against: the ETag the page read
// with the resource (ADR-043). A change to an older version is refused,
// so nobody overwrites what someone else changed meanwhile.
export function ifMatch(etag: string): RequestInit {
	return { headers: { 'If-Match': etag } };
}

// etagOf reads the version that an answer's ETag header names.
export function etagOf(headers: Headers): string {
	return headers.get('ETag') ?? '';
}
