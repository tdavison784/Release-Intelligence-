IMAGE_REGISTRY?=quay.io
IMAGE_NAMESPACE?=example
IMAGE_TAG?=latest
image:
	docker build -t $(IMAGE_REGISTRY)/$(IMAGE_NAMESPACE)/deployer-ci-builder:$(IMAGE_TAG) .
