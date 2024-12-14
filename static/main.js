import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';

var scene;
var renderer;
var camera;
var controls;
var mainMesh;

function setupThree()
{
	scene = new THREE.Scene();
	camera = new THREE.PerspectiveCamera( 60, window.innerWidth / window.innerHeight, 0.1, 100000 );
	camera.position.x = 1000;
	camera.position.y = 1000;
	camera.position.z = 1000;
	camera.up.set(0,0,1);

	renderer = new THREE.WebGLRenderer();
	renderer.setSize(window.innerWidth, window.innerHeight);
	renderer.setAnimationLoop(animate);
	renderer.localClippingEnabled = true;
	document.body.appendChild(renderer.domElement);

	controls = new OrbitControls( camera, renderer.domElement );

	const light = new THREE.AmbientLight(0x404040);
	scene.add(light);

	const directionalLight = new THREE.DirectionalLight( 0xffffff, 1 );
	directionalLight.position.set(-1, 1, 1).normalize();
	scene.add( directionalLight );

	window.addEventListener( 'resize', onWindowResize, false );
	function onWindowResize(){
		camera.aspect = window.innerWidth / window.innerHeight;
		camera.updateProjectionMatrix();
		renderer.setSize( window.innerWidth, window.innerHeight );
	}
}

async function getMesh()
{
	let response = await fetch("/heightmap");
	let json = await response.json();

	const geometry = new THREE.BufferGeometry();
	const vertices = new Float32Array(json.vertices);
	const indices = json.triangles;

	geometry.setIndex( indices );
	geometry.setAttribute( 'position', new THREE.BufferAttribute( vertices, 3 ) );
	geometry.doubleSided = true;
	geometry.computeVertexNormals();

	const material = new THREE.MeshPhongMaterial({color: 0xffffff, side: THREE.DoubleSide});
	material.flatShading = true;
	material.wireframe = false;
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
	console.log("Updated main mesh");
}

function animate()
{
	controls.update();
	renderer.render(scene, camera);
}

setupThree();
getMesh();
