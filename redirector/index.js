import { cert, key } from './ssl-config.js'; 

Bun.serve({
  port: 80,
  fetch(req) {
    return new Response('Please connect via SSH with your terminal: ssh letsgosky.social', { 
      status: 200,
      headers: { 'Content-Type': 'text/plain' }
    });
  }
});

Bun.serve({
  port: 443,
  tls: {
    key,
    cert,
  },
  fetch(req) {
    return new Response('Please connect via SSH with your terminal: ssh letsgosky.social', { 
      status: 200,
      headers: { 'Content-Type': 'text/plain' }
    });
  }
});

console.log('Web server running. Redirecting all traffic to SSH.');
