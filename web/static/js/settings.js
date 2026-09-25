// Viewer settings (not part of the generation config), persisted in the
// browser.

const storageKey = 'wgen.settings';

export const views = ['orbit', 'top', 'map'];
export const colors = ['terrain', 'height'];
export const shadings = ['lit', 'unlit'];

const defaults = {
	view: 'orbit',
	color: 'terrain',
	shading: 'lit',
	wireframe: false,
	riverPower: 0.5,
	riverWidth: 10,
	contours: 0,
	grid: 0
};

export function loadSettings() {
	let saved = {};
	try {
		saved = JSON.parse(localStorage.getItem(storageKey)) ?? {};
	} catch {
		// Ignore broken storage
	}

	const settings = { ...defaults };
	for (const key in defaults)
		if (typeof saved[key] === typeof defaults[key])
			settings[key] = saved[key];

	return settings;
}

export function storeSettings(settings) {
	localStorage.setItem(storageKey, JSON.stringify(settings));
}

// Next value of a cyclic setting
export function cycle(values, value) {
	return values[(values.indexOf(value) + 1) % values.length];
}

// Server render options for the overlay (rivers, contour lines, grid)
export function overlayOptions(settings) {
	return {
		riverPower: settings.riverPower,
		riverWidth: settings.riverWidth,
		contours: settings.contours,
		grid: settings.grid
	};
}
