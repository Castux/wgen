import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { GUI } from 'three/addons/libs/lil-gui.module.min.js';

var map;

var scene;
var renderer;
var cameras;
var controls;
var mainMesh;
var colorBuffers = [null, null];
var materials;

var config = {
	wireframe: false,
	color: 0,
	view: 0,
	shading: 0
};

const colorOptions = {terrain: 0, height: 1};
const viewOptions = {orbit: 0, top: 1};
const shadingOptions = {lit: 0, unlit: 1};

function setupThree()
{
	scene = new THREE.Scene();
	scene.background = new THREE.Color(156.0/255, 196.0/255, 240.0/255);

	var perspCamera = new THREE.PerspectiveCamera( 60, window.innerWidth / window.innerHeight, 0.1, 100000 );
	perspCamera.up.set(0,0,1);

	var orthoCamera = new THREE.OrthographicCamera( -1000, 1000, 1000, -1000, 0, 100000 );
	orthoCamera.position.set(0,0,1000);
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
		null
	];
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
		if (controls[i])
			controls[i].enabled = (i == index);
	}
}

function setupGui()
{
	const gui = new GUI();

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
}

async function getMesh()
{
	let responses = await Promise.all([
		fetch("/heightmap"),
		fetch("/colors")
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
	controls[config.view]?.update();
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
