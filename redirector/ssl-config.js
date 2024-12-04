import fs from 'fs';

export const key = fs.readFileSync('/etc/letsencrypt/live/letsgosky.social/privkey.pem');
export const cert = fs.readFileSync('/etc/letsencrypt/live/letsgosky.social/fullchain.pem');
