# How To Use — Fleet BMS API

This project lets bus cameras stream video to a server, and gives any app a
simple API to list buses, watch live video, and pull recent recordings.

No frontend is included — this is a pure JSON API. Build your own UI (web,
mobile, whatever) that calls it.

## 1. Start the server

```powershell
cd prototype
docker compose up -d --build
```

This starts two things:
- **MediaMTX** — receives camera video, records it, plays it back
- **Go backend** — the JSON API your app will talk to, on `http://localhost:4000`

Check it's running:

```powershell
curl http://localhost:4000/health
```

Should print `ok`.

## 2. Point cameras at the server

Each camera on a bus streams to a URL shaped like this:

```
rtmp://<server-ip>:11935/<BUS_ID>_<CAMERA_NUMBER>
```

- `BUS_ID` — the bus's ID, e.g. `DL1PC0001`. Use it exactly as-is.
- `CAMERA_NUMBER` — `1`, `2`, or `3` for that bus's cameras.

Example, bus `DL1PC0001` with 3 cameras:

```
rtmp://<server-ip>:11935/DL1PC0001_1
rtmp://<server-ip>:11935/DL1PC0001_2
rtmp://<server-ip>:11935/DL1PC0001_3
```

Any camera app that can push RTMP (or SRT/WHIP) works — no setup needed on
the server side, it just starts showing up once a camera connects.

## 3. Use the API

All requests go to `http://<server-ip>:4000`.

### See every bus and how many cameras are live

```
GET /api/fleet
```

```json
{
  "buses": [
    { "id": "DL1PC0001", "cams": [1, 2, 3], "lastSeen": 1783352685 },
    { "id": "DL1PC0002", "cams": [1], "lastSeen": 1783352685 }
  ],
  "totals": { "busesOnline": 2, "busesSeen": 2, "camsOnline": 4 }
}
```

`cams` lists which camera numbers are currently live. An empty list means
that bus was seen recently but isn't streaming right now.

### See detail for one bus

```
GET /api/bus/DL1PC0001
```

Returns bitrate, codec, and viewer count for each of that bus's cameras.

### Get a playable video link for a bus

```
GET /api/stream/DL1PC0001
```

```json
[
  { "cam": 1, "path": "DL1PC0001_1", "ready": true,
    "whepUrl": "/whep/DL1PC0001_1/whep",
    "hlsUrl": "/live/DL1PC0001_1/index.m3u8" }
]
```

- `whepUrl` — for low-latency live video (WebRTC). Use this first.
- `hlsUrl` — fallback, works everywhere, ~2-5 second delay.

Add `?cam=2` to get just one camera instead of all of them.

Play `hlsUrl` directly in any `<video>` tag with [hls.js](https://github.com/video-dev/hls.js),
or use a WHEP client library for `whepUrl`.

### Get a recording from the last hour

```
GET /api/stream/DL1PC0001/recording?from=2026-07-06T10:00:00Z&to=2026-07-06T10:02:00Z
```

```json
[
  { "cam": 1, "path": "DL1PC0001_1",
    "url": "/playback/get?path=DL1PC0001_1&start=2026-07-06T10:00:00Z&duration=120&format=mp4" }
]
```

Open `url` directly — it's a playable/downloadable mp4 clip. Only the
**last 1 hour** of video is kept; older windows return nothing.

Add `?cam=2` to get just one camera instead of all of them.

## 4. Quick cheat sheet

| I want to... | Call this |
|---|---|
| See all buses | `GET /api/fleet` |
| See one bus's cameras in detail | `GET /api/bus/{busId}` |
| Watch a bus live | `GET /api/stream/{busId}` |
| Watch one specific camera live | `GET /api/stream/{busId}?cam=2` |
| Watch a past moment | `GET /api/stream/{busId}/recording?from=...&to=...` |
| Check the server is alive | `GET /health` |

## 5. Things to know

- Bus IDs and camera counts are **not fixed** — the server figures out who's
  online by watching who's currently streaming. Nothing to register.
- Recordings only cover the **last hour**. Anything older is gone.
- This is a dev setup: no login/auth yet. Don't expose it to the public
  internet as-is — see `prototype/REMOTE_BUSES_SETUP.md` for production
  hardening notes.
- Full endpoint list is also always available by hitting the API root:
  `GET /` on `http://localhost:4000`.
