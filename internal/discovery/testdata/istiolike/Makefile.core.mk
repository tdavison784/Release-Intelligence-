# Docker hub to push images to
HUB ?=meshproj
TAG ?= $(shell git rev-parse --verify HEAD)
MESH_BASE_REGISTRY ?= registry.mesh.example/release
