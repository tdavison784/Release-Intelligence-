#!/usr/bin/env bash
INITIAL_RELEASE=$(git describe --tags)
kubectl apply -f "https://github.com/example/certmgr/releases/download/${INITIAL_RELEASE}/certmgr.yaml"
