# GitAgentic Node

HTTP-based gossip node for the GitAgentic decentralized network. Designed for Railway multi-region deployment.

## Deploy on Railway

Deploy this repo 3 times with different env vars for each region:

### Node 1 (US-East)
```
NODE_NAME=node.gitagentic.dev
NODE_REGION=US-EAST
NODE_FLAG=🇺🇸
PEERS=https://node2.gitagentic.dev,https://node3.gitagentic.dev
```

### Node 2 (EU-West)
```
NODE_NAME=node2.gitagentic.dev
NODE_REGION=EU-WEST
NODE_FLAG=🇪🇺
PEERS=https://node.gitagentic.dev,https://node3.gitagentic.dev
```

### Node 3 (Asia-Tokyo)
```
NODE_NAME=node3.gitagentic.dev
NODE_REGION=AP-TOKYO
NODE_FLAG=🇯🇵
PEERS=https://node.gitagentic.dev,https://node2.gitagentic.dev
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Node info (DID, version, region) |
| GET | `/health` | Health check |
| GET | `/api/v1/repos` | List repos on this node |
| POST | `/api/v1/repos` | Create repo |
| GET | `/api/v1/stats` | Node statistics |
| GET | `/api/v1/peers` | Connected peers |
| GET | `/api/v1/events` | Recent events |
| GET | `/api/v1/agents` | Registered agents |
| POST | `/gossip/push` | Receive gossip event from peer |
| GET | `/gossip/sync` | Sync state with peer |

## Gossip Protocol

Nodes communicate via HTTP POST. Every push/repo_create event is broadcast to all peers. Peers sync state every 30 seconds.

## License

Apache-2.0
