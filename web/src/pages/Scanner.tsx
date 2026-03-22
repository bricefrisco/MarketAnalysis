import { useEffect, useRef, useState } from 'react';
import {
  Box, Container, Typography, Paper, Divider, LinearProgress,
} from '@mui/material';
import WifiIcon from '@mui/icons-material/Wifi';
import WifiOffIcon from '@mui/icons-material/WifiOff';

const ZVZ_WEAPONS = [
  { name: 'Great Hammer',     baseId: '2H_HAMMER',               tiers: [7, 8] },
  { name: 'Heavy Mace',       baseId: '2H_MACE',                 tiers: [7, 8] },
  { name: 'Staff of Balance', baseId: '2H_ROCKSTAFF_KEEPER',     tiers: [7, 8] },
  { name: 'Oathkeepers',      baseId: '2H_DUALMACE_AVALON',      tiers: [7, 8] },
  { name: 'Lifecurse Staff',  baseId: 'MAIN_CURSEDSTAFF_UNDEAD', tiers: [7, 8] },
  { name: 'Taproot',          baseId: 'OFF_TOTEM_KEEPER',        tiers: [7, 8] },
  { name: 'Rotcaller Staff',  baseId: 'MAIN_CURSEDSTAFF_CRYSTAL', tiers: [7, 8] },
  { name: 'Rootbound Staff',  baseId: '2H_SHAPESHIFTER_SET2',    tiers: [7, 8] },
  { name: 'Carving Sword',    baseId: '2H_CLEAVER_HELL',         tiers: [7, 8, 9] },
  { name: 'Realmbreaker',     baseId: '2H_AXE_AVALON',           tiers: [7, 8, 9] },
  { name: 'Spiked Gauntlets', baseId: '2H_KNUCKLES_SET3',        tiers: [7, 8, 9] },
  { name: 'Battle Bracers',   baseId: '2H_KNUCKLES_SET2',        tiers: [7, 8, 9] },
  { name: 'Hellfire Hands',   baseId: '2H_KNUCKLES_HELL',        tiers: [7, 8, 9] },
  { name: 'Permafrost Prism', baseId: '2H_ICECRYSTAL_UNDEAD',    tiers: [7, 8, 9] },
  { name: 'Bloodletter',      baseId: 'MAIN_RAPIER_MORGANA',     tiers: [7, 8, 9] },
  { name: 'Facebreaker',      baseId: 'OFF_SPIKEDSHIELD_MORGANA', tiers: [7, 8, 9] },
];

// Quality levels tracked (Normal=1, Good=2, Outstanding=3, Excellent=4, Masterpiece=5)
const QUALITIES = [
  { level: 1, label: 'N' },
  { level: 2, label: 'G' },
  { level: 3, label: 'O' },
  { level: 4, label: 'E' },
  { level: 5, label: 'M' },
];

const QUALITY_COLORS: Record<number, string> = {
  1: '#9e9e9e',  // grey
  2: '#4ade80',  // green
  3: '#60a5fa',  // blue
  4: '#c084fc',  // purple
  5: '#f59e0b',  // amber/gold
};

// effective tier = base_tier + enchantment
function getCombos(baseId: string, effTier: number) {
  const combos = [];
  for (let enc = 0; enc <= 4; enc++) {
    const base = effTier - enc;
    if (base >= 4 && base <= 8) {
      combos.push({
        itemTypeId: `T${base}_${baseId}`,
        enchantment: enc,
        label: enc > 0 ? `${base}.${enc}` : `${base}`,
      });
    }
  }
  return combos;
}

function scanKey(itemTypeId: string, enchantment: number, quality: number) {
  return `${itemTypeId}|${enchantment}|${quality}`;
}

const TOTAL = ZVZ_WEAPONS.reduce(
  (acc, w) =>
    acc + w.tiers.reduce((a, t) => a + getCombos(w.baseId, t).length, 0) * QUALITIES.length,
  0,
);

export function Scanner() {
  const [scanned, setScanned] = useState<Set<string>>(new Set());
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${window.location.host}/ws/scanner`);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onmessage = (evt) => {
      try {
        const { item_type_id, enchantment_level, quality_level } = JSON.parse(evt.data) as {
          item_type_id: string;
          enchantment_level: number;
          quality_level: number;
        };
        setScanned((prev) =>
          new Set(prev).add(scanKey(item_type_id, enchantment_level, quality_level))
        );
      } catch { /* ignore malformed messages */ }
    };

    return () => ws.close();
  }, []);

  const pct = TOTAL > 0 ? (scanned.size / TOTAL) * 100 : 0;

  return (
    <Container maxWidth="xl" sx={{ py: 4 }}>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', mb: 3 }}>
        <Box>
          <Typography variant="h5" fontWeight={700} letterSpacing="-0.02em" sx={{ mb: 0.5 }}>
            ZVZ Market Scanner
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Open each item in-game to capture market data. Progress resets on page reload.
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
          {connected
            ? <WifiIcon sx={{ color: 'success.main', fontSize: 18 }} />
            : <WifiOffIcon sx={{ color: 'error.main', fontSize: 18 }} />}
          <Typography variant="caption" color={connected ? 'success.main' : 'error.main'} fontWeight={600}>
            {connected ? 'Live' : 'Disconnected'}
          </Typography>
        </Box>
      </Box>

      {/* Progress */}
      <Box sx={{ mb: 3 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.75 }}>
          <Typography variant="caption" color="text.secondary" fontWeight={600}>
            {scanned.size} / {TOTAL} scanned
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {pct.toFixed(0)}%
          </Typography>
        </Box>
        <LinearProgress
          variant="determinate"
          value={pct}
          sx={{ height: 6, borderRadius: 3, backgroundColor: 'rgba(255,255,255,0.06)' }}
        />
      </Box>

      {/* Quality legend */}
      <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
        {QUALITIES.map((q) => (
          <Box key={q.level} sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <Box sx={{
              width: 10, height: 10, borderRadius: '50%',
              backgroundColor: QUALITY_COLORS[q.level],
            }} />
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
              {q.level === 1 ? 'Normal' : q.level === 2 ? 'Good' : q.level === 3 ? 'Outstanding' : q.level === 4 ? 'Excellent' : 'Masterpiece'}
            </Typography>
          </Box>
        ))}
      </Box>

      {/* Weapon rows */}
      <Paper elevation={0} sx={{ overflow: 'hidden' }}>
        {ZVZ_WEAPONS.map((weapon, wi) => (
          <Box key={weapon.baseId}>
            {wi > 0 && <Divider />}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, px: 2.5, py: 1.5, flexWrap: 'wrap' }}>
              <Typography
                variant="body2" fontWeight={700}
                sx={{ minWidth: 148, color: 'text.primary', flexShrink: 0 }}
              >
                {weapon.name}
              </Typography>

              {weapon.tiers.map((eff) => {
                const combos = getCombos(weapon.baseId, eff);
                const totalForTier = combos.length * QUALITIES.length;
                const doneForTier = combos.reduce(
                  (a, c) => a + QUALITIES.filter((q) => scanned.has(scanKey(c.itemTypeId, c.enchantment, q.level))).length,
                  0,
                );
                return (
                  <Box key={eff} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="caption" fontWeight={700} sx={{
                      fontSize: '0.65rem', letterSpacing: '0.04em', flexShrink: 0,
                      color: doneForTier === totalForTier ? 'success.main' : 'text.disabled',
                    }}>
                      T{eff}
                    </Typography>

                    {combos.map((c) => (
                      <Box key={c.itemTypeId + c.enchantment} sx={{
                        display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 0.4,
                        px: 0.75, py: 0.5, borderRadius: 1,
                        border: '1px solid',
                        borderColor: QUALITIES.every((q) => scanned.has(scanKey(c.itemTypeId, c.enchantment, q.level)))
                          ? 'rgba(74,222,128,0.35)'
                          : 'rgba(255,255,255,0.07)',
                        backgroundColor: QUALITIES.every((q) => scanned.has(scanKey(c.itemTypeId, c.enchantment, q.level)))
                          ? 'rgba(74,222,128,0.06)'
                          : 'transparent',
                        transition: 'all 0.2s ease',
                        minWidth: 32,
                      }}>
                        <Typography variant="caption" sx={{
                          fontSize: '0.68rem', fontWeight: 700, lineHeight: 1,
                          fontVariantNumeric: 'tabular-nums',
                          color: QUALITIES.some((q) => scanned.has(scanKey(c.itemTypeId, c.enchantment, q.level)))
                            ? 'text.primary'
                            : 'text.disabled',
                        }}>
                          {c.label}
                        </Typography>
                        <Box sx={{ display: 'flex', gap: '3px' }}>
                          {QUALITIES.map((q) => {
                            const done = scanned.has(scanKey(c.itemTypeId, c.enchantment, q.level));
                            return (
                              <Box key={q.level} sx={{
                                width: 6, height: 6, borderRadius: '50%',
                                backgroundColor: done ? QUALITY_COLORS[q.level] : 'rgba(255,255,255,0.12)',
                                transition: 'background-color 0.2s ease',
                              }} />
                            );
                          })}
                        </Box>
                      </Box>
                    ))}
                  </Box>
                );
              })}
            </Box>
          </Box>
        ))}
      </Paper>
    </Container>
  );
}
