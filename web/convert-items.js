#!/usr/bin/env node
/**
 * Convert items.txt to items.json
 * Fetches items.txt from GitHub and converts to a lookup map
 */

import fs from 'fs';
import path from 'path';
import https from 'https';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const ITEMS_URL = 'https://raw.githubusercontent.com/ao-data/ao-bin-dumps/refs/heads/master/formatted/items.txt';
const OUTPUT_PATH = path.join(__dirname, 'src/assets/items.json');

function fetchItems() {
  return new Promise((resolve, reject) => {
    https.get(ITEMS_URL, (res) => {
      let data = '';
      res.on('data', (chunk) => {
        data += chunk;
      });
      res.on('end', () => {
        resolve(data);
      });
    }).on('error', reject);
  });
}

function parseItems(content) {
  const items = {};
  const lines = content.split('\n');

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;

    // Format: "Number : ItemID : Display Name"
    const parts = trimmed.split(':');
    if (parts.length >= 3) {
      const itemId = parts[1].trim();
      const displayName = parts[2].trim();
      // Map ItemID -> Display Name (e.g., "T4_CAPEITEM_FW_BRIDGEWATCH@2" -> "Fiend Armor Cape")
      items[itemId] = displayName;
    }
  }

  return items;
}

async function main() {
  try {
    console.log('Fetching items from GitHub...');
    const content = await fetchItems();
    console.log('Parsing items...');
    const items = parseItems(content);
    console.log(`Converted ${Object.keys(items).length} items`);

    console.log(`Writing to ${OUTPUT_PATH}`);
    fs.writeFileSync(OUTPUT_PATH, JSON.stringify(items, null, 2));
    console.log('✓ Done!');
  } catch (error) {
    console.error('Error:', error.message);
    process.exit(1);
  }
}

main();
