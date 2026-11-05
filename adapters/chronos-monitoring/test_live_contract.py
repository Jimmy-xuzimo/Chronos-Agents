import sys,unittest
from pathlib import Path
from flask import Flask,jsonify,request
sys.path.insert(0,str(Path(__file__).parent))
from chronos_hub import Hub,install
class Integration(unittest.TestCase):
 def test_authenticated_envelope_order_and_history(self):
  app=Flask(__name__)
  old=[{'id':'server'},{'id':'nas-fnos','name':'old nas'},{'id':'windows-xzm'}]
  def auth(f):
   def wrap(*a,**k):
    if request.headers.get('X-Test-Auth')!='yes':return jsonify(error='unauthorized'),401
    return f(*a,**k)
   return wrap
  app.add_url_rule('/api/nodes','nodes',lambda:jsonify(items=old))
  app.add_url_rule('/api/stats','stats',lambda:jsonify(source={'provider':'old'}))
  app.add_url_rule('/api/history','history',lambda:jsonify(points=[{'ts':1,'cpu':20},{'ts':3,'cpu':22}]))
  app.add_url_rule('/api/overview','overview',lambda:jsonify(nodes=old,node_id=request.args.get('node','server'),stats={'data':{'old':True}}))
  h=Hub('https://hub.example.com','fictional-token-'*4,{'nas-fnos':'nas-hub'})
  device={'id':'nas-hub','name':'NAS','kind':'nas','state':'recent','received_at':1767225600,'latest':{'sampled_at':'2026-01-01T00:00:00Z','metrics':{}}}
  h.devices=lambda:{'nas-fnos':device}
  h.history=lambda *_:{'points':[{'ts':3,'cpu':30},{'ts':4,'cpu':40}],'source':{'provider':'chronos-hub'}}
  install(app,auth,h);c=app.test_client();headers={'X-Test-Auth':'yes'}
  for path in ['/api/nodes','/api/stats','/api/overview','/api/history']:
   self.assertEqual(c.get(path).status_code,401)
  r=c.get('/api/overview?node=nas-fnos',headers=headers).json
  self.assertEqual([x['id'] for x in r['nodes']],['server','nas-fnos','windows-xzm'])
  self.assertEqual(r['stats']['status'],'ok');self.assertIn('mem',r['stats']['data'])
  self.assertTrue(c.get('/api/overview',headers=headers).json['stats']['data']['old'])
  points=c.get('/api/history?node=nas-fnos',headers=headers).json['points']
  self.assertEqual([x['ts'] for x in points],[1,3,4]);self.assertEqual(points[1]['cpu'],30)
  self.assertEqual(c.get('/api/stats?node=nas-fnos',headers=headers).json['source']['status'],'ok')
if __name__=='__main__':unittest.main()
