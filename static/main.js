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

var config = {
	wireframe: false,
	color: 0,
	camera: 0,

	topView: resetTopView
};

const colorOptions = {terrain: 0, height: 1};
const cameraOptions = {perspective: 0, orthographic: 1};

function setupThree()
{
	scene = new THREE.Scene();
	scene.background = new THREE.Color(156.0/255, 196.0/255, 240.0/255);

	var perspCamera = new THREE.PerspectiveCamera( 60, window.innerWidth / window.innerHeight, 0.1, 100000 );
	perspCamera.up.set(0,0,1);

	var orthoCamera = new THREE.OrthographicCamera( -1000, 1000, 1000, -1000, 0, 100000 );
	orthoCamera.position.set(0,0,1000);
	orthoCamera.up.set(0,0,1);
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
	activateControl(config.camera);

	const light = new THREE.AmbientLight(0xffffff, 1);
	scene.add(light);

	const directionalLight = new THREE.DirectionalLight(0xffffff, 3);
	directionalLight.position.set(-1, 1, 1).normalize();
	scene.add(directionalLight);

	window.addEventListener( 'resize', onWindowResize, false );
	function onWindowResize() {
		const aspect = window.innerWidth / window.innerHeight;
		cameras[0].aspect = aspect;
		cameras[0].updateProjectionMatrix();

		cameras[1].left = -map.width / 2.0 * aspect;
		cameras[1].right = map.width / 2.0 * aspect;
		cameras[1].bottom = -map.height / 2.0;
		cameras[1].top = map.height / 2.0;
		cameras[1].updateProjectionMatrix();

		renderer.setSize( window.innerWidth, window.innerHeight );
	}
}

function activateControl(index)
{
	for(var i = 0; i < controls.length; i++)
	{
		controls[i].enabled = (i == index);
	}
}

function setupGui()
{
	const gui = new GUI();

	gui.add(config, 'wireframe').onChange(function(value) {
		mainMesh.material.wireframe = value;
	});

	gui.add(config, 'color', colorOptions).onChange(function(value) {
		mainMesh.geometry.setAttribute('color', colorBuffers[value]);
	});

	gui.add(config, 'camera', cameraOptions).onChange(function(value) {
		activateControl(value);
	});

	gui.add(config, 'topView').name("Top view");
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

	const material = new THREE.MeshLambertMaterial();
	material.flatShading = true;
	material.wireframe = false;
	material.vertexColors = true;
	material.clippingPlanes = [
		new THREE.Plane( new THREE.Vector3(1, 0, 0), map.width / 2.0),
		new THREE.Plane( new THREE.Vector3(-1, 0, 0), map.width / 2.0),
		new THREE.Plane( new THREE.Vector3(0, 1, 0), map.height / 2.0),
		new THREE.Plane( new THREE.Vector3(0, -1, 0), map.height / 2.0)
	];

	mainMesh = new THREE.Mesh(geometry, material);
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

	console.log("Updated main mesh");
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

	var f32buffer = new Float32Array(colors);
	var index = colorOptions.terrain;

	if (colorBuffers[index] == undefined)
		colorBuffers[index] = new THREE.BufferAttribute(f32buffer, 3);
	else
		colorBuffers[index].array = f32buffer;
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

	var f32buffer = new Float32Array(normalizedZ);
	var index = colorOptions.height;

	if (colorBuffers[index] == undefined)
		colorBuffers[index] = new THREE.BufferAttribute(f32buffer, 3);
	else
		colorBuffers[index].array = f32Buffer;
}

function animate()
{
	controls[config.camera].update();
	render();
}

function render()
{
	renderer.render(scene, cameras[config.camera]);
}

function resetTopView()
{
	cameras[config.camera].position.set(0, 0, Math.max(map.width, map.height));
	cameras[config.camera].zoom = 1.0;
	cameras[config.camera].updateWorldMatrix();
	cameras[config.camera].updateProjectionMatrix();

	controls[config.camera].target = new THREE.Vector3(0,0,0);
	controls[config.camera].update();
}

setupThree();
setupGui();
getMesh();
