const http = require('node:http');

// Contract: gank-image-grabber/README.md and live /api/extract on 2026-10-26.
http.createServer((req, res) => {
  const request = new URL(req.url, 'http://localhost');
  const sourceUrl = request.searchParams.get('url');
  const send = (status, value) => {
    res.writeHead(status, {'Content-Type': 'application/json'});
    res.end(JSON.stringify(value));
  };
  if (req.method !== 'GET' || request.pathname !== '/api/extract') {
    return send(501, {error: 'Endpoint not modeled'});
  }
  if (!sourceUrl) return send(400, {error: 'Missing url query parameter.'});
  if (sourceUrl.endsWith('/empty')) return send(422, {error: 'No image media found on the page.'});
  if (sourceUrl.endsWith('/unavailable')) return send(500, {error: 'Upstream request failed.'});
  send(200, {
    pageType: 'post', title: 'Fixture', sourceUrl, imageCount: 2,
    images: [
      {index: 1, type: 'image', renderedUrl: 'https://lh3.googleusercontent.com/fixture', originalUrl: 'https://lh3.googleusercontent.com/fixture=s0', thumbUrl: 'https://lh3.googleusercontent.com/fixture', isPrimary: false, description: ''},
      {index: 2, type: 'image', renderedUrl: 'https://media.ganknow.com/public/PM.fixture.jpg', originalUrl: 'https://media.ganknow.com/public/PM.fixture.jpg', thumbUrl: 'https://media.ganknow.com/public/PM.fixture.jpg', isPrimary: false, description: ''}
    ]
  });
}).listen(process.env.PORT || 4911);
