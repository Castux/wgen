import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { GUI } from 'three/addons/libs/lil-gui.module.min.js';
import { LineMaterial } from 'three/addons/lines/LineMaterial.js';
import { LineSegmentsGeometry } from 'three/addons/lines/LineSegmentsGeometry.js';
import { LineSegments2 } from 'three/addons/lines/LineSegments2.js';

var map;

var scene;
var renderer;
var cameras;
var controls;
var mainMesh;
var colorBuffers = [null, null];
var materials;
var riverMesh;

var config = {
	wireframe: false,
	color: 0,
	view: 0,
	shading: 0,
	edgeWidth: 1,
	elevation: 0,
	grid: 0
};

const colorOptions = {terrain: 0, height: 1};
const viewOptions = {orbit: 0, top: 1};
const shadingOptions = {lit: 0, unlit: 1};

function setupThree()
{
	scene = new THREE.Scene();
	scene.background = new THREE.Color(156.0/255, 196.0/255, 240.0/255);

	var perspCamera = new THREE.PerspectiveCamera( 60, window.innerWidth / window.innerHeight, 1, 10000 );
	perspCamera.up.set(0,0,1);

	var orthoCamera = new THREE.OrthographicCamera( -1000, 1000, 1000, -1000, 0, 10000 );
	orthoCamera.position.set(0,10,1000);
	orthoCamera.up.set(0,1,0);
	orthoCamera.lookAt(0,0,0);

	cameras = [perspCamera, orthoCamera];

	renderer = new THREE.WebGLRenderer({antialias: true});
	renderer.setSize(window.innerWidth, window.innerHeight);
	renderer.setAnimationLoop(animate);
	renderer.localClippingEnabled = true;
	document.body.appendChild(renderer.domElement);

	controls = [
		new OrbitControls(perspCamera, renderer.domElement),
		new OrbitControls(orthoCamera, renderer.domElement)
	];
	controls[1].enableRotate = false;
	activateControl(config.view);

	const light = new THREE.AmbientLight(0xffffff, 1);
	scene.add(light);

	const directionalLight = new THREE.DirectionalLight(0xffffff, 3);
	directionalLight.position.set(-1, 1, 1).normalize();
	scene.add(directionalLight);

	materials = [
		new THREE.MeshLambertMaterial(),
		new THREE.MeshBasicMaterial()
	];

	materials.forEach(material => {
		material.flatShading = true;
		material.wireframe = false;
		material.vertexColors = true;
	});

	window.addEventListener('resize', onWindowResize, false);
	document.addEventListener('keydown', onKeyDown);
}

function activateControl(index)
{
	for(var i = 0; i < controls.length; i++)
	{
		controls[i].enabled = (i == index);
		console.log(i, index);
	}
}

function setupGui()
{
	const gui = new GUI();
	gui.title("Settings");

	gui.add(config, 'wireframe')
		.name("Wireframe (w)")
		.onChange(updateWireframe)
		.listen();

	gui.add(config, 'color', colorOptions)
		.name("Color (shift)")
		.onChange(updateColors)
		.listen();

	gui.add(config, 'view', viewOptions)
		.name("View (tab)")
		.onChange(activateControl)
		.listen();

	gui.add(config, 'shading', shadingOptions)
		.name("Shading (q)")
		.onChange(updateShading)
		.listen();

	gui.add(config, 'edgeWidth', 0, 5)
		.name("Edge width")
		.onChange(v => riverShader.uniforms.edgeWidth.value = v);

	gui.add(config, 'elevation', 0, 100, 1)
		.name("Elevation lines")
		.onChange(v => riverShader.uniforms.elevation.value = v);

	gui.add(config, 'grid', 0, 100, 1)
		.name("Grid size")
		.onChange(v => riverShader.uniforms.grid.value = v);
}

async function getMesh()
{
	let responses = await Promise.all([
		fetch("/heightmap"),
		fetch("/colors"),
		fetch("/rivers")
	]);
	map = await responses[0].json();

	updateTerrainColors(await responses[1].json());
	updateHeightColors();

	const geometry = new THREE.BufferGeometry();

	geometry.setIndex(map.triangles);
	geometry.setAttribute('position', new THREE.Float32BufferAttribute(map.vertices, 3));
	geometry.setAttribute('color', colorBuffers[config.color]);
	geometry.computeVertexNormals();

	mainMesh = new THREE.Mesh(geometry, materials[0]);
	mainMesh.translateX(-map.width / 2.0);
	mainMesh.translateY(-map.height / 2.0);
	scene.add(mainMesh);

	const aspect = window.innerWidth / window.innerHeight;

	cameras[0].position.set(0.0, -map.height, Math.max(map.width, map.height));
	cameras[0].far = Math.max(map.width, map.height) * 2.25;
	cameras[0].updateProjectionMatrix();

	controls[0].maxDistance = Math.max(map.width, map.height) * 1.5;

	cameras[1].position.set(0.0, 0.0, Math.max(map.width, map.height));
	cameras[1].left = -map.width / 2.0 * aspect;
	cameras[1].right = map.width / 2.0 * aspect;
	cameras[1].bottom = -map.height / 2.0;
	cameras[1].top = map.height / 2.0;
	cameras[1].updateProjectionMatrix();

	materials.forEach(material =>
		material.clippingPlanes = [
			new THREE.Plane( new THREE.Vector3(1, 0, 0), map.width / 2.0),
			new THREE.Plane( new THREE.Vector3(-1, 0, 0), map.width / 2.0),
			new THREE.Plane( new THREE.Vector3(0, 1, 0), map.height / 2.0),
			new THREE.Plane( new THREE.Vector3(0, -1, 0), map.height / 2.0)
		]
	);

	updateRivers(await responses[2].json());

	console.log("Updated main mesh");
}

function setColorBuffer(index, f32buffer)
{
	if (!colorBuffers[index])
		colorBuffers[index] = new THREE.BufferAttribute(f32buffer, 3);
	else
		colorBuffers[index].array = f32buffer;
}

function updateTerrainColors(colorsJson)
{
	var colors = [];
	for(var i = 0; i < colorsJson.length ; i++)
	{
		var color = new THREE.Color(colorsJson[i]);
		colors.push(color.r);
		colors.push(color.g);
		colors.push(color.b);
	}

	setColorBuffer(colorOptions.terrain, new Float32Array(colors));
}

function updateHeightColors()
{
	var normalizedZ = [];
	for(var i = 2; i < map.vertices.length; i += 3)
	{
		const z = (map.vertices[i] - map.lowest) / (map.highest - map.lowest);
		normalizedZ.push(z);
		normalizedZ.push(z);
		normalizedZ.push(z);
	}

	setColorBuffer(colorOptions.height, new Float32Array(normalizedZ));
}

const riverShader = new THREE.ShaderMaterial({
	uniforms: {
		edgeWidth: { value: 1.0 },
		elevation: { value: 0.0 },
		grid: { value: 0.0 },
		clipping: { value: new THREE.Vector2() }
	},

	vertexShader:`
		attribute vec3 distToEdge;
		varying vec3 vEdgeDist;
		varying vec3 vPos;

		void main()
		{
			gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
			vEdgeDist = distToEdge;
			vPos = position;
		}`,

	fragmentShader: `

		uniform float edgeWidth;
		uniform float elevation;
		uniform float grid;
		uniform vec2 clipping;

		varying vec3 vEdgeDist;
		varying vec3 vPos;

		void main()
		{
			if (vPos.x < 0.0 || vPos.x > clipping[0] ||
				vPos.y < 0.0 || vPos.y > clipping[1])
				discard;

			float river = 1e10;
			if (vEdgeDist.x > 0.0 && river > vEdgeDist.x)
				river = vEdgeDist.x;
			if (vEdgeDist.y > 0.0 && river > vEdgeDist.y)
				river = vEdgeDist.y;
			if (vEdgeDist.z > 0.0 && river > vEdgeDist.z)
				river = vEdgeDist.z;

			vec3 color;
			vec3 frac = mod(vPos, grid);
			float distToGrid = min(frac.x, frac.y);
			float zfrac = mod(vPos.z, elevation);

			if (edgeWidth > 0.0 && (river < edgeWidth))
				color = vec3(0,0,1);

			else if (zfrac <= 0.5)
				color = vec3(0,1,0);

			else if (distToGrid <= 0.1)
				color = vec3(0,0,0);

			else
				discard;

			gl_FragColor = vec4(color, 1.0);
		}`
});

function updateRivers(json)
{
	var position = [];
	var distToEdge = [];

	function riverFlow(i,j)
	{
		if (json[i * 2] == j)
			return json[i * 2 + 1]

		if (json[j * 2] == i)
			return json[j * 2 + 1]

		return 0.0;
	}

	for (var i = 0; i < map.triangles.length; i += 3)
	{
		var vertices = [];
		var vertexIndices = [];
		for (var v = 0; v < 3; v++)
		{
			const vertexIndex = map.triangles[i + v];
			const vertex = new THREE.Vector3(
				map.vertices[vertexIndex * 3 + 0],
				map.vertices[vertexIndex * 3 + 1],
				map.vertices[vertexIndex * 3 + 2]
			)

			position.push(vertex.x, vertex.y, vertex.z);
			vertices.push(vertex);
			vertexIndices.push(vertexIndex);
		}

		for (var v = 0; v < 3; v++)
		{
			var p = vertices[v];
			var a = vertices[(v + 1) % 3];
			var b = vertices[(v + 2) % 3];

			var flow = riverFlow(vertexIndices[(v + 1) % 3], vertexIndices[(v + 2) % 3]);

			const nx = b.x - a.x;
			const ny = b.y - a.y;
			const nd = Math.sqrt(nx * nx + ny * ny);
			var d = ((p.y - a.y) * nx - (p.x - a.x) * ny) / nd;

			d /= Math.sqrt(flow);

			if (flow == 0.0)
				d = -1.0;

			distToEdge.push(v == 0 ? d : 0.0);
			distToEdge.push(v == 1 ? d : 0.0);
			distToEdge.push(v == 2 ? d : 0.0);
		}
	}

	var geometry = new THREE.BufferGeometry();
	geometry.setAttribute('position', new THREE.Float32BufferAttribute(position, 3));
	geometry.setAttribute('distToEdge', new THREE.Float32BufferAttribute(distToEdge, 3));

	riverShader.uniforms.clipping.value = new THREE.Vector2(map.width, map.height);

	riverMesh = new THREE.Mesh(geometry, riverShader);
	riverMesh.translateZ(0.75);

	mainMesh.add(riverMesh);
}

function updateColors()
{
	mainMesh.geometry.setAttribute('color', colorBuffers[config.color]);
}

function updateWireframe()
{
	materials.forEach(m => m.wireframe = config.wireframe);
}

function updateShading()
{
	mainMesh.material = materials[config.shading];
}

function animate()
{
	controls[config.view].update();

	if (riverMesh && cameras[config.view])
		riverMesh.position.z = Math.max(0.1, cameras[config.view].position.z / 100.0);
	render();
}

function render()
{
	renderer.render(scene, cameras[config.view]);
}

function onWindowResize()
{
	const aspect = window.innerWidth / window.innerHeight;
	cameras[0].aspect = aspect;
	cameras[0].updateProjectionMatrix();

	cameras[1].left = -map.width / 2.0 * aspect;
	cameras[1].right = map.width / 2.0 * aspect;
	cameras[1].bottom = -map.height / 2.0;
	cameras[1].top = map.height / 2.0;
	cameras[1].updateProjectionMatrix();

	renderer.setSize(window.innerWidth, window.innerHeight);
}

function onKeyDown(event)
{
	switch(event.key)
	{
		case "Tab":
			config.view = (config.view + 1) % Object.keys(viewOptions).length;
			activateControl(config.view);
			event.preventDefault();
			break;
		case "Shift":
			config.color = (config.color + 1) % Object.keys(colorOptions).length;
			updateColors();
			break;
		case "w":
			config.wireframe = !config.wireframe;
			updateWireframe();
			break;
		case "q":
			config.shading = (config.shading + 1) % Object.keys(shadingOptions).length;
			updateShading();
			break;
	}
}

setupThree();
setupGui();
getMesh();
