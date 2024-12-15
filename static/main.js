import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { GUI } from 'three/addons/libs/lil-gui.module.min.js';

var scene;
var renderer;
var camera;
var orthoCamera;
var controls;
var mainMesh;
var terrainColors;
var heightColors;

var config = {
	wireframe: false,
	color: 0
};

function setupThree()
{
	scene = new THREE.Scene();
	scene.background = new THREE.Color(156.0/255, 196.0/255, 240.0/255);

	camera = new THREE.PerspectiveCamera( 60, window.innerWidth / window.innerHeight, 0.1, 100000 );
	camera.up.set(0,0,1);

	orthoCamera = new THREE.PerspectiveCamera( -1000, 1000, 1000, -1000, 0, 100000 );
	orthoCamera.up.set(0,1,0);
	orthoCamera.position.set(0,0,10000);
	orthoCamera.lookAt(0,0,0);

	renderer = new THREE.WebGLRenderer({antialias: true});
	renderer.setSize(window.innerWidth, window.innerHeight);
	renderer.setAnimationLoop(animate);
	renderer.localClippingEnabled = true;
	document.body.appendChild(renderer.domElement);

	controls = new OrbitControls(camera, renderer.domElement);

	const light = new THREE.AmbientLight(0xffffff, 1);
	scene.add(light);

	const directionalLight = new THREE.DirectionalLight(0xffffff, 3);
	directionalLight.position.set(-1, 1, 1).normalize();
	scene.add(directionalLight);

	window.addEventListener( 'resize', onWindowResize, false );
	function onWindowResize(){
		camera.aspect = window.innerWidth / window.innerHeight;
		camera.updateProjectionMatrix();
		renderer.setSize( window.innerWidth, window.innerHeight );
	}
}

function setupGui()
{
	const gui = new GUI();

	gui.add(config, 'wireframe').onChange(function(value) {
		mainMesh.material.wireframe = value;
	});
	gui.add(config, 'color', {terrain: 0, height: 1}).onChange(function(value) {
		mainMesh.geometry.setAttribute('color', value == 0 ? terrainColors : heightColors);
	});
}

async function getMesh()
{
	let responses = await Promise.all([
		fetch("/heightmap"),
		fetch("/colors")
	]);
	let json = await responses[0].json();
	let colorsJson = await responses[1].json();

	const geometry = new THREE.BufferGeometry();

	var colors = [];
	for(var i = 0; i < colorsJson.length ; i++)
	{
		var color = new THREE.Color(colorsJson[i]);
		colors.push(color.r);
		colors.push(color.g);
		colors.push(color.b);
	}
	terrainColors = new THREE.Float32BufferAttribute(colors, 3);

	var normalizedZ = [];
	for(var i = 2; i < json.vertices.length; i += 3)
	{
		const z = (json.vertices[i] - json.lowest) / (json.highest - json.lowest);
		normalizedZ.push(z);
		normalizedZ.push(z);
		normalizedZ.push(z);
	}
	heightColors = new THREE.Float32BufferAttribute(normalizedZ, 3);

	geometry.setIndex(json.triangles);
	geometry.setAttribute('position', new THREE.Float32BufferAttribute(json.vertices, 3));
	geometry.setAttribute('color', terrainColors);
	geometry.computeVertexNormals();

	const material = new THREE.MeshLambertMaterial();
	material.flatShading = true;
	material.wireframe = false;
	material.vertexColors = true;
	material.clippingPlanes = [
		new THREE.Plane( new THREE.Vector3(1, 0, 0), json.width / 2.0),
		new THREE.Plane( new THREE.Vector3(-1, 0, 0), json.width / 2.0),
		new THREE.Plane( new THREE.Vector3(0, 1, 0), json.height / 2.0),
		new THREE.Plane( new THREE.Vector3(0, -1, 0), json.height / 2.0)
	];

	mainMesh = new THREE.Mesh(geometry, material);
	mainMesh.translateX(-json.width / 2.0);
	mainMesh.translateY(-json.height / 2.0);
	scene.add(mainMesh);

	camera.position.set(0, 0, json.width / 2.0);
	orthoCamera.position.set(0, 0, 1000);
	orthoCamera.left = -json.width / 2.0;
	orthoCamera.right = json.width / 2.0;
	orthoCamera.bottom = json.height / 2.0;
	orthoCamera.top = -json.height / 2.0;
	orthoCamera.updateProjectionMatrix();
	orthoCamera.updateMatrixWorld();

	console.log("Updated main mesh");
}

function animate()
{
	controls.update();
	render();
}

function render()
{
	renderer.render(scene, camera);
}

setupThree();
setupGui();
getMesh();
