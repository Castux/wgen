// Communication with the wgen server. See internal/server/server.go for the
// API.

async function request(url, options) {
	const response = await fetch(url, options);
	const isJSON = response.headers.get('Content-Type')?.startsWith('application/json');
	const body = isJSON ? await response.json() : null;

	if (!response.ok)
		throw new Error(body?.error ?? `${response.status} ${response.statusText}`);

	return body;
}

function post(url, body) {
	return request(url, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body)
	});
}

export const getConfig = () => request('/api/config');
export const getSchema = () => request('/api/schema');
export const patchConfig = patch => post('/api/config', patch);
export const saveConfig = () => post('/api/save');
export const exportWorld = () => post('/api/export');

// URL of a rendered image (see renderOptions in server.go). The version only
// serves to bust the browser cache.
export function renderURL(options, version) {
	const params = new URLSearchParams({ ...options, v: version });
	return `/api/render.png?${params}`;
}

// Calls onState with every engine state, and with null when the connection
// is lost (the browser reconnects by itself).
export function subscribe(onState) {
	const events = new EventSource('/api/events');
	events.onmessage = event => onState(JSON.parse(event.data));
	events.onerror = () => onState(null);
	return events;
}

// Fetches and decodes the binary mesh (see encodeMesh in server.go).
export async function getMesh() {
	const response = await fetch('/api/mesh');
	if (!response.ok)
		throw new Error(`mesh: ${response.status} ${response.statusText}`);

	const buffer = await response.arrayBuffer();
	const view = new DataView(buffer);

	const magic = new TextDecoder().decode(new Uint8Array(buffer, 0, 4));
	if (magic !== 'WGM1')
		throw new Error(`mesh: bad format ${magic}`);

	const numVertices = view.getUint32(8, true);
	const numTriangles = view.getUint32(12, true);

	let offset = 32;
	const take = (Type, count) => {
		const array = new Type(buffer, offset, count);
		offset += count * Type.BYTES_PER_ELEMENT;
		return array;
	};

	return {
		version: view.getUint32(4, true),
		width: view.getFloat32(16, true),
		height: view.getFloat32(20, true),
		lowest: view.getFloat32(24, true),
		highest: view.getFloat32(28, true),
		positions: take(Float32Array, 3 * numVertices),
		indices: take(Uint32Array, 3 * numTriangles),
		colors: take(Uint8Array, 4 * numVertices)
	};
}
