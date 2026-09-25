// 3D view of the terrain mesh: vertex colors (terrain or height), flat
// shading, and the server-rendered overlay (rivers, contours, grid) as a
// texture.

import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';

function srgbToLinear(c) {
	return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

export class TerrainView {
	constructor(parent) {
		this.renderer = new THREE.WebGLRenderer({ antialias: true });
		this.renderer.setPixelRatio(window.devicePixelRatio);
		this.renderer.localClippingEnabled = true;
		parent.appendChild(this.renderer.domElement);

		this.scene = new THREE.Scene();
		this.scene.background = new THREE.Color(156 / 255, 196 / 255, 240 / 255);

		this.scene.add(new THREE.AmbientLight(0xffffff, 1));
		const light = new THREE.DirectionalLight(0xffffff, 3);
		light.position.set(-1, 1, 1).normalize();
		this.scene.add(light);

		this.materials = {
			lit: new THREE.MeshLambertMaterial(),
			unlit: new THREE.MeshBasicMaterial()
		};
		for (const material of Object.values(this.materials)) {
			material.flatShading = true;
			material.vertexColors = true;
		}

		this.setupCameras();

		this.mesh = null;
		this.size = null;
		this.colorBuffers = {};
		this.settings = null;
	}

	setupCameras() {
		const canvas = this.renderer.domElement;

		const perspective = new THREE.PerspectiveCamera(60, 1, 1, 10000);
		perspective.up.set(0, 0, 1);

		const ortho = new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 10000);
		ortho.up.set(0, 1, 0);

		this.cameras = { orbit: perspective, top: ortho };
		this.controls = {
			orbit: new OrbitControls(perspective, canvas),
			top: new OrbitControls(ortho, canvas)
		};
		this.controls.top.enableRotate = false;
	}

	// Frames the whole map in both cameras
	resetCameras() {
		if (!this.size)
			return;

		const { width, height } = this.size;
		const extent = Math.max(width, height);

		const perspective = this.cameras.orbit;
		perspective.position.set(0, -height, extent);
		perspective.far = extent * 2.25;
		this.controls.orbit.target.set(0, 0, 0);
		this.controls.orbit.maxDistance = extent * 1.5;

		const ortho = this.cameras.top;
		ortho.position.set(0, 0, extent);
		ortho.zoom = 1;
		this.controls.top.target.set(0, 0, 0);

		this.resize();
		this.controls.orbit.update();
		this.controls.top.update();
	}

	resize() {
		const canvas = this.renderer.domElement;
		const width = canvas.parentElement.clientWidth;
		const height = canvas.parentElement.clientHeight;
		if (width === 0 || height === 0)
			return;

		this.renderer.setSize(width, height, false);
		const aspect = width / height;

		this.cameras.orbit.aspect = aspect;
		this.cameras.orbit.updateProjectionMatrix();

		// Fit the map height, or its width if that's the tighter constraint
		if (this.size) {
			const ortho = this.cameras.top;
			const half = Math.max(this.size.height, this.size.width / aspect) / 2;
			ortho.left = -half * aspect;
			ortho.right = half * aspect;
			ortho.top = half;
			ortho.bottom = -half;
			ortho.updateProjectionMatrix();
		}
	}

	setMesh(data) {
		const { width, height } = data;
		const sizeChanged = !this.size || this.size.width !== width || this.size.height !== height;
		this.size = { width, height };

		const geometry = new THREE.BufferGeometry();
		geometry.setIndex(new THREE.BufferAttribute(data.indices, 1));
		geometry.setAttribute('position', new THREE.BufferAttribute(data.positions, 3));

		const numVertices = data.positions.length / 3;
		const uv = new Float32Array(2 * numVertices);
		const terrain = new Float32Array(3 * numVertices);
		const elevation = new Float32Array(3 * numVertices);
		const range = data.highest - data.lowest || 1;

		for (let i = 0; i < numVertices; i++) {
			uv[2 * i] = data.positions[3 * i] / width;
			uv[2 * i + 1] = data.positions[3 * i + 1] / height;

			for (let k = 0; k < 3; k++)
				terrain[3 * i + k] = srgbToLinear(data.colors[4 * i + k] / 255);

			const z = (data.positions[3 * i + 2] - data.lowest) / range;
			elevation[3 * i] = elevation[3 * i + 1] = elevation[3 * i + 2] = z;
		}

		geometry.setAttribute('uv', new THREE.BufferAttribute(uv, 2));
		this.colorBuffers = {
			terrain: new THREE.BufferAttribute(terrain, 3),
			height: new THREE.BufferAttribute(elevation, 3)
		};

		if (this.mesh) {
			this.scene.remove(this.mesh);
			this.mesh.geometry.dispose();
		}

		this.mesh = new THREE.Mesh(geometry);
		this.mesh.position.set(-width / 2, -height / 2, 0);
		this.scene.add(this.mesh);

		// Hide the margin around the map
		const planes = [
			new THREE.Plane(new THREE.Vector3(1, 0, 0), width / 2),
			new THREE.Plane(new THREE.Vector3(-1, 0, 0), width / 2),
			new THREE.Plane(new THREE.Vector3(0, 1, 0), height / 2),
			new THREE.Plane(new THREE.Vector3(0, -1, 0), height / 2)
		];
		for (const material of Object.values(this.materials))
			material.clippingPlanes = planes;

		if (sizeChanged)
			this.resetCameras();

		if (this.settings)
			this.applySettings(this.settings);
	}

	async setOverlay(url) {
		const texture = await new THREE.TextureLoader().loadAsync(url);
		texture.colorSpace = THREE.SRGBColorSpace;
		texture.anisotropy = this.renderer.capabilities.getMaxAnisotropy();

		for (const material of Object.values(this.materials)) {
			const old = material.map;
			material.map = texture;
			material.needsUpdate = true;
			if (old && old !== texture)
				old.dispose();
		}
	}

	// Overlay resolution: detailed enough for close ups, within GPU limits
	overlayScale() {
		const maxSize = Math.min(8192, this.renderer.capabilities.maxTextureSize);
		const extent = this.size ? Math.max(this.size.width, this.size.height) : 2048;
		return Math.min(4, maxSize / 2 / extent);
	}

	applySettings(settings) {
		this.settings = settings;

		for (const material of Object.values(this.materials))
			material.wireframe = settings.wireframe;

		if (!this.mesh)
			return;

		this.mesh.material = this.materials[settings.shading];
		this.mesh.geometry.setAttribute('color', this.colorBuffers[settings.color]);

		for (const [view, control] of Object.entries(this.controls))
			control.enabled = view === settings.view;
	}

	show(visible) {
		this.renderer.domElement.style.display = visible ? '' : 'none';
		this.renderer.setAnimationLoop(visible ? () => this.render() : null);
		if (visible)
			this.resize();
	}

	render() {
		const view = this.settings?.view === 'top' ? 'top' : 'orbit';
		this.controls[view].update();
		this.renderer.render(this.scene, this.cameras[view]);
	}
}
