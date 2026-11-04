import unittest
from chronos_hub import Hub, stats


class MappingTests(unittest.TestCase):
    def test_unavailable_is_not_zero(self):
        device = {'state': 'recent', 'latest': {'sampled_at': '2026-01-01T00:00:00Z', 'metrics': {
            'cpu': {'status': 'unsupported', 'scope': 'host', 'data': None},
            'memory': {'status': 'supported', 'data': {'percent': 25, 'total_bytes': 1000, 'used_bytes': 250, 'available_bytes': 750}},
        }}}
        data = stats(device)
        self.assertIsNone(data['cpu']['total'])
        self.assertEqual(data['mem']['used'], 250)
        self.assertEqual(data['capabilities']['cpu']['status'], 'unsupported')
        self.assertIsNone(data['net']['rx'])
        self.assertEqual(data['sampled_at'], 1767225600)

    def test_https_and_stable_bindings(self):
        for origin in ('http://example.com', 'https://u:p@example.com', 'https://example.com/path', 'https://example.com?token=x'):
            with self.assertRaises(ValueError):
                Hub(origin, 'fictional-credential-' * 3, {})
        with self.assertRaises(ValueError):
            Hub('https://hub.example.com', 'fictional-credential-' * 3, {'a': 'same-id', 'b': 'same-id'})

    def test_missing_device_keeps_placeholder(self):
        hub = Hub('https://hub.example.com', 'fictional-credential-' * 3, {'stable-old-id': 'new-hub-device-id'})
        hub._get = lambda _: {'items': []}
        nodes = hub.nodes()
        self.assertEqual(nodes[0]['id'], 'stable-old-id')
        self.assertEqual(nodes[0]['source_state'], 'binding_unavailable')
        self.assertIsNone(nodes[0]['cpu'])

    def test_cache_reused_without_duplicate_fetch(self):
        hub = Hub('https://hub.example.com', 'fictional-credential-' * 3, {})
        calls = []
        def get(path):
            calls.append(path)
            return {'items': []}
        hub._get = get
        hub.devices(); hub.devices()
        self.assertEqual(len(calls), 1)


if __name__ == '__main__':
    unittest.main()
