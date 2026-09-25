// Settings panel: viewer settings, generation parameters (built from the
// server's schema) and actions.

import { GUI } from 'three/addons/libs/lil-gui.module.min.js';
import { views, colors, shadings } from './settings.js';

export class Panel {
	// handlers: onSetting(key, value), onParam(path, value), onSave(),
	// onExport(), onResetView()
	constructor(settings, handlers) {
		this.handlers = handlers;
		this.gui = new GUI({ title: 'wgen' });

		this.buildViewFolder(settings);
		this.buildActions();

		this.paramsFolder = this.gui.addFolder('Generation');
		this.schemaKey = null;
		this.values = {};
		this.controllers = [];
	}

	buildViewFolder(settings) {
		const folder = this.gui.addFolder('View');
		const on = key => value => this.handlers.onSetting(key, value);

		folder.add(settings, 'view', views).name('View (tab)').onChange(on('view')).listen();
		folder.add(settings, 'color', colors).name('Color (shift)').onChange(on('color')).listen();
		folder.add(settings, 'shading', shadings).name('Shading (q)').onChange(on('shading')).listen();
		folder.add(settings, 'wireframe').name('Wireframe (w)').onChange(on('wireframe')).listen();

		// Server rendered: only update when done dragging
		folder.add(settings, 'riverWidth', 0, 20, 0.1).name('River max width').onFinishChange(on('riverWidth'));
		folder.add(settings, 'riverPower', 0, 1, 0.01).name('River width growth').onFinishChange(on('riverPower'));
		folder.add(settings, 'contours', 0, 100, 1).name('Contour interval').onFinishChange(on('contours'));
		folder.add(settings, 'grid', 0, 500, 10).name('Grid size').onFinishChange(on('grid'));
	}

	buildActions() {
		const actions = {
			save: () => this.handlers.onSave(),
			export: () => this.handlers.onExport(),
			reset: () => this.handlers.onResetView()
		};

		const folder = this.gui.addFolder('Actions');
		this.saveButton = folder.add(actions, 'save').name('Save config');
		folder.add(actions, 'export').name('Export files');
		folder.add(actions, 'reset').name('Reset view');
		this.setDirty(false);
	}

	setDirty(dirty) {
		this.saveButton.name(dirty ? 'Save config (unsaved changes)' : 'Save config');
		this.saveButton.enable(dirty);
	}

	// Rebuilds the parameter controls if the schema changed, and shows the
	// config values.
	setConfig(schema, config) {
		const key = JSON.stringify(schema);
		if (key !== this.schemaKey) {
			this.schemaKey = key;
			this.buildParams(schema);
		}

		for (const { param, id, controller } of this.controllers) {
			const value = param.path.reduce((object, k) => object?.[k], config);
			if (value !== undefined && value !== this.values[id]) {
				this.values[id] = value;
				controller.updateDisplay();
			}
		}
	}

	buildParams(schema) {
		for (const child of [...this.paramsFolder.children])
			child.destroy();
		this.controllers = [];
		this.values = {};

		const folders = {};
		for (const param of schema) {
			if (!folders[param.group]) {
				folders[param.group] = this.paramsFolder.addFolder(param.group);
				if (param.group.startsWith('Terrain: ') || param.group === 'Export')
					folders[param.group].close();
			}
			const folder = folders[param.group];

			const id = param.path.join('.');
			this.values[id] = param.type === 'bool' ? false : param.type === 'enum' ? param.options[0] : 0;

			let controller;
			switch (param.type) {
				case 'bool':
					controller = folder.add(this.values, id);
					break;
				case 'enum':
					controller = folder.add(this.values, id, param.options);
					break;
				default:
					controller = folder.add(this.values, id, param.min, param.max, param.step);
			}

			controller.name(param.label).onFinishChange(value => this.handlers.onParam(param.path, value));
			this.controllers.push({ param, id, controller });
		}
	}
}
