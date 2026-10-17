// The fetch the generated client sends every request through. The session
// cookie goes along on the interface's own origin (ADR-033), so no request
// carries a token. Every answer comes back with its status, problems
// included, and the caller decides what a status means.
export const apiFetch = async <T>(url: string, init: RequestInit): Promise<T> => {
	const response = await fetch(url, { ...init, credentials: 'same-origin' });
	const text = await response.text();
	const data: unknown = text === '' ? undefined : JSON.parse(text);
	return { data, status: response.status, headers: response.headers } as T;
};
