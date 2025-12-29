#!/bin/bash

cd $(dirname $0)/../

docker compose -f .devcontainer/docker-compose.yml down
docker volume prune -f -a
docker compose -f .devcontainer/docker-compose.yml up -d
