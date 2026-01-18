#!/bin/bash

cd $(dirname $0)/../deployment/dev

docker compose down
docker volume prune -f -a
docker compose up -d
