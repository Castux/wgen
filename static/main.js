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
	camera = new THREE.PerspectiveCamera( 75, window.innerWidth / window.innerHeight, 0.1, 100000 );
	camera.position.x = 1000;
	camera.position.y = 1000;
	camera.position.z = 1000;
	camera.up.set(0,0,1);

	renderer = new THREE.WebGLRenderer();
	renderer.setSize( window.innerWidth, window.innerHeight );
	renderer.setAnimationLoop( animate );
	document.body.appendChild( renderer.domElement );

	controls = new OrbitControls( camera, renderer.domElement );

	const directionalLight = new THREE.DirectionalLight( 0xffffff, 0.5 );
	directionalLight.position.set( - 1, 0, 1 ).normalize();
	scene.add( directionalLight );
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

	const material = new THREE.MeshBasicMaterial( { color: 0xff0000 } );
	material.wireframe = true;
	mainMesh = new THREE.Mesh( geometry, material );
	mainMesh.translateX(-json.width / 2.0);
	mainMesh.translateY(-json.height / 2.0);

	scene.add(mainMesh);
	console.log("Updated main mesh");
}

function animate()
{
	controls.update();
	renderer.render( scene, camera );
}

setupThree();
getMesh();
