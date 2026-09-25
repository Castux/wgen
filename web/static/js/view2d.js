// 2D view: the server-rendered map, with pan (drag) and zoom (wheel).
// Double click to fit the map to the window.

// Largest image side requested from the server
const maxImageSize = 4096;

export class MapView {
	constructor(parent, onScaleChange) {
		this.canvas = document.createElement('canvas');
		this.canvas.className = 'map';
		parent.appendChild(this.canvas);
		this.context = this.canvas.getContext('2d');

		this.onScaleChange = onScaleChange;
		this.size = null;   // world size
		this.image = null;
		this.zoom = 1;      // screen pixels per world unit
		this.offset = { x: 0, y: 0 };
		this.visible = false;
		this.needsFit = false;

		this.setupEvents();
	}

	setupEvents() {
		const canvas = this.canvas;
		let drag = null;

		canvas.addEventListener('pointerdown', event => {
			drag = { x: event.clientX, y: event.clientY };
			canvas.setPointerCapture(event.pointerId);
			canvas.classList.add('dragging');
		});

		canvas.addEventListener('pointermove', event => {
			if (!drag)
				return;
			this.offset.x += event.clientX - drag.x;
			this.offset.y += event.clientY - drag.y;
			drag = { x: event.clientX, y: event.clientY };
			this.draw();
		});

		const endDrag = () => {
			drag = null;
			canvas.classList.remove('dragging');
		};
		canvas.addEventListener('pointerup', endDrag);
		canvas.addEventListener('pointercancel', endDrag);

		canvas.addEventListener('wheel', event => {
			event.preventDefault();
			const factor = Math.exp(-event.deltaY * 0.002);
			this.zoomAt(event.offsetX, event.offsetY, factor);
		}, { passive: false });

		canvas.addEventListener('dblclick', () => this.fit());
	}

	zoomAt(x, y, factor) {
		const zoom = Math.min(Math.max(this.zoom * factor, this.fitZoom() / 4), 64);
		factor = zoom / this.zoom;

		// Keep the point under the cursor fixed
		this.offset.x = x - (x - this.offset.x) * factor;
		this.offset.y = y - (y - this.offset.y) * factor;
		this.zoom = zoom;

		this.draw();
		this.onScaleChange();
	}

	fitZoom() {
		if (!this.size)
			return 1;
		const { clientWidth, clientHeight } = this.canvas;
		return Math.min(clientWidth / this.size.width, clientHeight / this.size.height);
	}

	fit() {
		if (!this.size)
			return;

		// Can't fit while hidden: do it when shown
		if (!this.visible) {
			this.needsFit = true;
			return;
		}
		this.needsFit = false;

		const { clientWidth, clientHeight } = this.canvas;
		this.zoom = this.fitZoom();
		this.offset.x = (clientWidth - this.size.width * this.zoom) / 2;
		this.offset.y = (clientHeight - this.size.height * this.zoom) / 2;
		this.draw();
		this.onScaleChange();
	}

	setSize(width, height) {
		const changed = !this.size || this.size.width !== width || this.size.height !== height;
		this.size = { width, height };
		if (changed)
			this.fit();
	}

	// Image scale (relative to the outline image) worth requesting at the
	// current zoom: a power of two, so that zooming doesn't rerender all the
	// time.
	imageScale() {
		if (!this.size || !(this.zoom > 0))
			return 1;
		const wanted = this.zoom * window.devicePixelRatio;
		const max = maxImageSize / Math.max(this.size.width, this.size.height);
		return Math.min(max, Math.pow(2, Math.ceil(Math.log2(wanted))));
	}

	// Loads a new image, keeping the current one displayed until it's ready
	setImage(url) {
		return new Promise((resolve, reject) => {
			const image = new Image();
			image.onload = () => {
				this.image = image;
				this.draw();
				resolve();
			};
			image.onerror = () => reject(new Error('could not load map image'));
			image.src = url;
		});
	}

	resize() {
		const ratio = window.devicePixelRatio;
		this.canvas.width = this.canvas.clientWidth * ratio;
		this.canvas.height = this.canvas.clientHeight * ratio;
		this.draw();
	}

	show(visible) {
		this.visible = visible;
		this.canvas.style.display = visible ? '' : 'none';
		if (visible) {
			this.resize();
			if (this.needsFit)
				this.fit();
		}
	}

	draw() {
		if (!this.visible)
			return;

		const ratio = window.devicePixelRatio;
		const context = this.context;
		context.setTransform(1, 0, 0, 1, 0, 0);
		context.clearRect(0, 0, this.canvas.width, this.canvas.height);

		if (!this.image || !this.size)
			return;

		context.setTransform(ratio, 0, 0, ratio, 0, 0);
		context.imageSmoothingEnabled = this.zoom * ratio < this.image.width / this.size.width * 2;
		context.drawImage(this.image, this.offset.x, this.offset.y,
			this.size.width * this.zoom, this.size.height * this.zoom);
	}
}
