// Viewer entry point: wires the server state to the views and the panel.
//
// The server pushes its state (version, busy, error...) on every change. When
// the version changes, the mesh and rendered images are fetched again.

import * as api from './api.js';
import { loadSettings, storeSettings, cycle, views, colors, shadings, overlayOptions } from './settings.js';
import { TerrainView } from './view3d.js';
import { MapView } from './view2d.js';
import { Panel } from './panel.js';

const settings = loadSettings();
const container = document.getElementById('views');
const statusElement = document.getElementById('status');

const terrainView = new TerrainView(container);
const mapView = new MapView(container, () => refreshMap());
const panel = new Panel(settings, {
	onSetting: (key, value) => updateSetting(key, value),
	onParam: (path, value) => run(api.patchConfig(toPatch(path, value))),
	onSave: () => run(api.saveConfig(), 'Config saved'),
	onExport: () => run(api.exportWorld().then(r => `Exported ${r.files.join(', ') || 'nothing (all exports disabled)'}`)),
	onResetView: () => settings.view === 'map' ? mapView.fit() : terrainView.resetCameras()
});

let serverState = null;
let loadedVersion = -1;
let message = null;      // last action result or error
let pending = 0;         // loads in progress

// Status bar

function showStatus() {
	const lines = [];
	const line = (text, cls) => lines.push(`<span class="${cls ?? ''}">${escape(text)}</span>`);

	if (!serverState)
		line('Disconnected from the server, retrying…', 'error');
	else {
		if (serverState.busy)
			line('Generating…', 'busy');
		else if (pending > 0)
			line('Loading…', 'busy');
		if (serverState.error)
			line(serverState.error, 'error');
		if (serverState.dirty)
			line('Unsaved config changes', 'dirty');
	}
	if (message)
		line(message.text, message.error ? 'error' : '');

	statusElement.innerHTML = lines.join('\n');
	statusElement.style.display = lines.length ? '' : 'none';
}

function escape(text) {
	const div = document.createElement('div');
	div.textContent = text;
	return div.innerHTML;
}

// Runs an action, showing its result or error
async function run(promise, success) {
	try {
		const result = await promise;
		message = typeof result === 'string' ? { text: result } : success ? { text: success } : null;
	} catch (error) {
		message = { text: error.message, error: true };
	}
	showStatus();
}

// Tracks a load for the status bar, reporting errors
async function load(promise) {
	pending++;
	showStatus();
	try {
		await promise;
	} catch (error) {
		message = { text: error.message, error: true };
	} finally {
		pending--;
		showStatus();
	}
}

// Nested JSON patch for a config value: ["terrains", "sea", "gradient"], -0.2
// gives {"terrains": {"sea": {"gradient": -0.2}}}
function toPatch(path, value) {
	return path.reduceRight((inner, key) => ({ [key]: inner }), value);
}

// Server state

api.subscribe(state => {
	serverState = state;
	panel.setDirty(state?.dirty ?? false);
	if (state && state.ready && !state.busy && state.version !== loadedVersion)
		refreshAll(state.version);
	if (state && !state.busy)
		load(refreshConfig());
	showStatus();
});

async function refreshConfig() {
	const [schema, config] = await Promise.all([api.getSchema(), api.getConfig()]);
	panel.setConfig(schema, config);
}

// Fetches the mesh and images of a version. Results of an older refresh
// arriving late are dropped.
async function refreshAll(version) {
	loadedVersion = version;
	message = null;

	await load(api.getMesh().then(mesh => {
		if (version !== loadedVersion)
			return;
		terrainView.setMesh(mesh);
		mapView.setSize(mesh.width, mesh.height);
	}));

	refreshOverlay();
	refreshMap();
}

let overlayURL = null;
function refreshOverlay() {
	if (loadedVersion < 0)
		return;
	const url = api.renderURL({ ...overlayOptions(settings), base: 'none', scale: terrainView.overlayScale() }, loadedVersion);
	if (url === overlayURL)
		return;
	overlayURL = url;
	load(terrainView.setOverlay(url));
}

// Only loaded when visible, since it depends on the zoom
let mapURL = null;
function refreshMap() {
	if (loadedVersion < 0 || settings.view !== 'map')
		return;
	const url = api.renderURL({
		...overlayOptions(settings),
		base: settings.color,
		shading: settings.shading === 'lit' ? 1 : 0,
		scale: mapView.imageScale()
	}, loadedVersion);
	if (url === mapURL)
		return;
	mapURL = url;
	load(mapView.setImage(url));
}

// Settings

function updateSetting(key, value) {
	settings[key] = value;
	storeSettings(settings);

	terrainView.applySettings(settings);
	if (key === 'view')
		showView();

	if (['riverWidth', 'riverPower', 'contours', 'grid'].includes(key))
		refreshOverlay();
	refreshMap();
}

function showView() {
	const map = settings.view === 'map';
	terrainView.show(!map);
	mapView.show(map);
	refreshMap();
}

document.addEventListener('keydown', event => {
	if (event.target.closest?.('.lil-gui'))
		return;

	switch (event.key) {
		case 'Tab':
			event.preventDefault();
			updateSetting('view', cycle(views, settings.view));
			break;
		case 'Shift':
			updateSetting('color', cycle(colors, settings.color));
			break;
		case 'w':
			updateSetting('wireframe', !settings.wireframe);
			break;
		case 'q':
			updateSetting('shading', cycle(shadings, settings.shading));
			break;
	}
});

window.addEventListener('resize', () => {
	terrainView.resize();
	mapView.resize();
});

terrainView.applySettings(settings);
showView();
showStatus();
