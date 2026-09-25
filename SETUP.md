# Setup local

## Pré-requisitos

- Docker em execução (com Docker Compose plugin)
- [Kind](https://kind.sigs.k8s.io/)
- `kubectl`
- `make`, `curl` e Bash

Go e Pulumi não precisam estar instalados na máquina: os comandos usam
containers descartáveis para ambos. O primeiro uso baixa as imagens Docker
necessárias.

## Deploy padrão no Kind

```sh
./scripts/ci.sh
```

O comando cria/reutiliza o cluster Kind e o registry local, executa `pulumi
preview`, aplica o plano, cria um port-forward para o Echo Service e faz uma
requisição de demonstração. Em um terminal interativo, o serviço continua
disponível em `http://localhost:8081` até `Ctrl-C`.

## Checagens completas antes do deploy

```sh
./scripts/ci.sh --ci
```

Além do fluxo padrão, executa lint, testes unitários, testes de infraestrutura,
testes de integração, Postman e scans de segurança.

## Observabilidade opcional

```sh
./scripts/ci.sh --observability
```

Esse modo habilita o stack opcional via Pulumi: Grafana, Loki, Mimir, Tempo e
OpenTelemetry Collector. O Grafana é port-forwarded para
`http://localhost:3001` e já inclui o dashboard **Echo Service Overview**
com logs, taxa de erros HTTP 5xx, latência p95 e taxa de requisições.

Para rodar todas as checagens e a observabilidade:

```sh
./scripts/ci.sh --ci --observability
```

## Comandos úteis

```sh
make pulumi-preview  # mostra o plano sem aplicar recursos
make forward          # Echo Service em http://localhost:8081
make forward-grafana  # Grafana em http://localhost:3001
make down             # remove recursos Pulumi, Kind e registry local
./scripts/ci.sh --help
```
