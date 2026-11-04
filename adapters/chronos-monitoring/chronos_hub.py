"""Opt-in, server-side read-only adapter. Never contains production settings."""
import datetime as dt
import json
import ssl
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def metric(sample, name):
    item = (sample or {}).get('metrics', {}).get(name, {})
    return item.get('data') or {} if item.get('status') == 'supported' else {}


def timestamp(value):
    try:
        return dt.datetime.fromisoformat(value.replace('Z', '+00:00')).timestamp()
    except (ValueError, TypeError, AttributeError):
        return None


def stats(device):
    sample = device.get('latest') or {}
    cpu, mem, swap, net, system, storage = [metric(sample, key) for key in ('cpu', 'memory', 'swap', 'network', 'system', 'storage')]
    volumes = storage.get('volumes', [])
    return {
        'cpu': {'total': cpu.get('percent'), 'percpu': cpu.get('per_core_percent', [])},
        'mem': {'percent': mem.get('percent'), 'total': mem.get('total_bytes'), 'used': mem.get('used_bytes'), 'available': mem.get('available_bytes')},
        'swap': {'percent': swap.get('percent'), 'total': swap.get('total_bytes'), 'used': swap.get('used_bytes')},
        'net': {'rx': net.get('rx_bytes_per_second'), 'tx': net.get('tx_bytes_per_second'),
                'total_rx': sum(i.get('rx_bytes', 0) for i in net.get('interfaces', [])) if net else None,
                'total_tx': sum(i.get('tx_bytes', 0) for i in net.get('interfaces', [])) if net else None,
                'conns': {'est': None, 'lsn': None},
                'ifaces': [{'name': i.get('name'), 'rx': i.get('rx_bytes_per_second'), 'tx': i.get('tx_bytes_per_second'), 'total_rx': i.get('rx_bytes'), 'total_tx': i.get('tx_bytes'), 'addresses': i.get('addresses', [])} for i in net.get('interfaces', [])]},
        # Block-device I/O is not assigned to a filesystem without a proven map.
        'disks': [{'mount': v.get('name'), 'device': v.get('name'), 'percent': v.get('percent'), 'total': v.get('total_bytes'), 'used': v.get('used_bytes'), 'io': {'r': None, 'w': None}} for v in volumes],
        'disk_io': storage.get('io'),
        'processes': [{'pid': p.get('pid'), 'name': p.get('name'), 'cpu': p.get('cpu_percent'), 'memory': p.get('memory_bytes')} for p in metric(sample, 'processes').get('items', [])],
        'system': {'os': system.get('os_name'), 'kernel': system.get('kernel_version'), 'hostname': system.get('hostname'), 'cpu_cores': cpu.get('cores'), 'uptime': system.get('uptime_seconds'), 'load': [], 'public_ip': None, 'cpu_model': None, 'procs': None},
        'sampled_at': timestamp(sample.get('sampled_at')),
        'capabilities': {key: {'status': value.get('status'), 'scope': value.get('scope')} for key, value in sample.get('metrics', {}).items()},
        'source': {'provider': 'chronos-hub', 'status': device.get('state'), 'updated': timestamp(sample.get('sampled_at')), 'received': device.get('received_at')},
    }


class Hub:
    def __init__(self, origin, token, bindings, ca_file=None):
        url = urllib.parse.urlsplit(origin)
        if url.scheme != 'https' or not url.hostname or url.username or url.password or url.query or url.fragment or url.path not in ('', '/'):
            raise ValueError('HTTPS root origin required')
        if not 32 <= len(token) <= 200 or any(c in token for c in '\r\n'):
            raise ValueError('invalid reader credential')
        if len(set(bindings.values())) != len(bindings) or not all(isinstance(k, str) and isinstance(v, str) for k, v in bindings.items()):
            raise ValueError('each panel node must map to one stable Hub device ID')
        self.origin, self._token, self.bindings = origin.rstrip('/'), token, dict(bindings)
        self._opener = urllib.request.build_opener(NoRedirect, urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=ca_file)))
        self._lock, self._data, self._at, self._error = threading.Lock(), {}, 0, False

    @classmethod
    def from_file(cls, path):
        value = json.loads(Path(path).read_text('utf-8'))
        return cls(value['origin'], value['reader_token'], value['bindings'], value.get('ca_file'))

    def _get(self, path):
        req = urllib.request.Request(self.origin + path, headers={'Authorization': 'Bearer ' + self._token, 'Accept': 'application/json'})
        with self._opener.open(req, timeout=8) as response:
            if response.status != 200:
                raise ValueError('Hub response rejected')
            raw = response.read(2 * 1024 * 1024 + 1)
            if len(raw) > 2 * 1024 * 1024:
                raise ValueError('Hub response too large')
            return json.loads(raw)

    def devices(self):
        with self._lock:
            if time.monotonic() - self._at >= 10:
                try:
                    self._data = {d['id']: d for d in self._get('/api/v1/devices')['items']}
                    self._error = False
                except urllib.error.HTTPError as error:
                    self._error = True
                    if error.code in (401, 403):
                        self._data = {}  # Never retain data across revoked scopes.
                except (OSError, ValueError, KeyError):
                    self._error = True
                self._at = time.monotonic()
            result = {}
            for alias, device_id in self.bindings.items():
                device = self._data.get(device_id)
                if device:
                    result[alias] = dict(device, state='source_error' if self._error else device['state'])
                else:
                    result[alias] = {'id': device_id, 'name': alias, 'kind': 'external', 'state': 'binding_unavailable', 'latest': None, 'received_at': None}
            return result

    def nodes(self):
        result = []
        for alias, device in self.devices().items():
            data = stats(device)
            result.append({'id': alias, 'hub_id': device['id'], 'name': device['name'], 'kind': device['kind'],
                           'status': 'online' if device['state'] == 'recent' else 'offline', 'source_state': device['state'],
                           'updated': data['sampled_at'], 'cores': data['system']['cpu_cores'], 'memory': data['mem']['total'],
                           'cpu': data['cpu']['total'], 'mem': data['mem']['percent'],
                           'disk': max((v['percent'] for v in data['disks'] if v['percent'] is not None), default=None),
                           'rx': data['net']['rx'], 'tx': data['net']['tx'], 'uptime': data['system']['uptime']})
        return result

    def history(self, alias, span):
        if span not in ('1h', '24h', '7d', '30d'):
            raise ValueError('invalid history range')
        device_id = self.bindings[alias]
        data = self._get('/api/v1/history?' + urllib.parse.urlencode({'device': device_id, 'range': span}))
        # Existing frontend keys retained; uncollected series remain null.
        return {'points': [dict(p, disk=None, read=None, write=None) for p in data['points']],
                'range': span, 'resolution_seconds': data['resolution_seconds'], 'source': {'provider': 'chronos-hub'}}


def install(app, authenticated, hub):
    """Call once after original Chronos routes are registered, before app.run.

    Reuses its login decorator. No new public route, duplicate collector, or DB.
    Existing nodes not in bindings retain their original source and history.
    """
    from flask import jsonify, make_response, request
    originals = {key: app.view_functions[key] for key in ('nodes', 'stats', 'history', 'overview')}

    @authenticated
    def nodes():
        response = make_response(originals['nodes']())
        if response.status_code != 200:
            return response
        items = response.get_json()['items']
        return jsonify(items=[n for n in items if n['id'] not in hub.bindings] + hub.nodes())

    @authenticated
    def device_stats():
        alias = request.args.get('node', 'server')
        if alias not in hub.bindings:
            return originals['stats']()
        device = hub.devices().get(alias)
        return (jsonify(**stats(device)) if device and device.get('latest') else (jsonify(error='hub_source_unavailable'), 503))

    @authenticated
    def history():
        alias = request.args.get('node', 'server')
        if alias not in hub.bindings:
            return originals['history']()
        try:
            return jsonify(hub.history(alias, request.args.get('range', '24h')))
        except (OSError, ValueError, KeyError):
            return jsonify(error='hub_history_unavailable'), 502

    @authenticated
    def overview():
        response = make_response(originals['overview']())
        if response.status_code != 200:
            return response
        data = response.get_json()
        data['nodes'] = [n for n in data.get('nodes', []) if n['id'] not in hub.bindings] + hub.nodes()
        alias = data.get('node_id', 'server')
        if alias in hub.bindings:
            device = hub.devices().get(alias)
            data['stats'] = stats(device) if device and device.get('latest') else None
            data['collector'] = {'provider': 'chronos-hub', 'status': device.get('state') if device else 'source_unavailable'}
        return jsonify(data)

    app.view_functions.update(nodes=nodes, stats=device_stats, history=history, overview=overview)
