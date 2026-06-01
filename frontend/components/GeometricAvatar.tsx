"use client";

/** Deterministic HSL color from a username string */
function hashHue(username: string): number {
  let hash = 0;
  for (let i = 0; i < username.length; i++) {
    hash = username.charCodeAt(i) + ((hash << 5) - hash);
  }
  return Math.abs(hash) % 360;
}

function hsl(h: number, s: number, l: number) {
  return `hsl(${h},${s}%,${l}%)`;
}

type ShapeIndex = 0 | 1 | 2 | 3 | 4;

function shapeIndex(username: string): ShapeIndex {
  let hash = 0;
  for (let i = 0; i < username.length; i++) {
    hash = (hash * 31 + username.charCodeAt(i)) >>> 0;
  }
  return (hash % 5) as ShapeIndex;
}

interface ShapeProps {
  hue: number;
  size: number;
}

function HexagonShape({ hue, size }: ShapeProps) {
  const s = size;
  const cx = s / 2;
  const cy = s / 2;
  const r = s * 0.38;
  const pts = Array.from({ length: 6 }, (_, i) => {
    const a = (Math.PI / 3) * i - Math.PI / 6;
    return `${cx + r * Math.cos(a)},${cy + r * Math.sin(a)}`;
  }).join(" ");
  const ri = r * 0.55;
  const ptsI = Array.from({ length: 6 }, (_, i) => {
    const a = (Math.PI / 3) * i - Math.PI / 6;
    return `${cx + ri * Math.cos(a)},${cy + ri * Math.sin(a)}`;
  }).join(" ");
  return (
    <>
      <polygon
        points={pts}
        fill="none"
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.04}
        opacity={0.5}
      />
      <polygon
        points={ptsI}
        fill={hsl(hue, 65, 25)}
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.03}
        opacity={0.7}
      />
      <circle cx={cx} cy={cy} r={s * 0.12} fill={hsl(hue, 80, 60)} opacity={0.6} />
    </>
  );
}

function DiamondShape({ hue, size }: ShapeProps) {
  const s = size;
  const cx = s / 2;
  const cy = s / 2;
  const r = s * 0.38;
  const pts = `${cx},${cy - r} ${cx + r * 0.75},${cy} ${cx},${cy + r} ${cx - r * 0.75},${cy}`;
  return (
    <>
      <polygon
        points={pts}
        fill="none"
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.04}
        opacity={0.5}
      />
      <polygon
        points={`${cx},${cy - r * 0.55} ${cx + r * 0.45},${cy} ${cx},${cy + r * 0.55} ${cx - r * 0.45},${cy}`}
        fill={hsl(hue, 65, 25)}
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.03}
        opacity={0.7}
      />
      <circle cx={cx} cy={cy} r={s * 0.1} fill={hsl(hue, 80, 60)} opacity={0.6} />
    </>
  );
}

function SquareShape({ hue, size }: ShapeProps) {
  const s = size;
  const cx = s / 2;
  const cy = s / 2;
  const half = s * 0.3;
  const rx = s * 0.07;
  return (
    <>
      <rect
        x={cx - half}
        y={cy - half}
        width={half * 2}
        height={half * 2}
        rx={rx}
        fill="none"
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.04}
        opacity={0.5}
      />
      <rect
        x={cx - half * 0.55}
        y={cy - half * 0.55}
        width={half * 1.1}
        height={half * 1.1}
        rx={rx * 0.6}
        fill={hsl(hue, 65, 25)}
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.03}
        opacity={0.7}
      />
      <circle cx={cx} cy={cy} r={s * 0.1} fill={hsl(hue, 80, 60)} opacity={0.6} />
    </>
  );
}

function TriangleShape({ hue, size }: ShapeProps) {
  const s = size;
  const cx = s / 2;
  const cy = s / 2;
  const r = s * 0.38;
  const pts = `${cx},${cy - r} ${cx + r * 0.87},${cy + r * 0.5} ${cx - r * 0.87},${cy + r * 0.5}`;
  const ri = r * 0.52;
  const ptsI = `${cx},${cy - ri} ${cx + ri * 0.87},${cy + ri * 0.5} ${cx - ri * 0.87},${cy + ri * 0.5}`;
  return (
    <>
      <polygon
        points={pts}
        fill="none"
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.04}
        opacity={0.5}
      />
      <polygon
        points={ptsI}
        fill={hsl(hue, 65, 25)}
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.03}
        opacity={0.7}
      />
      <circle cx={cx} cy={cy} r={s * 0.1} fill={hsl(hue, 80, 60)} opacity={0.6} />
    </>
  );
}

function CircleShape({ hue, size }: ShapeProps) {
  const s = size;
  const cx = s / 2;
  const cy = s / 2;
  const r = s * 0.36;
  return (
    <>
      <circle
        cx={cx}
        cy={cy}
        r={r}
        fill="none"
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.04}
        strokeDasharray={`${s * 0.08} ${s * 0.05}`}
        opacity={0.45}
      />
      <circle
        cx={cx}
        cy={cy}
        r={r * 0.6}
        fill={hsl(hue, 65, 25)}
        stroke={hsl(hue, 70, 55)}
        strokeWidth={s * 0.03}
        opacity={0.7}
      />
      <circle cx={cx} cy={cy} r={s * 0.1} fill={hsl(hue, 80, 60)} opacity={0.6} />
    </>
  );
}

const SHAPES = [HexagonShape, DiamondShape, SquareShape, TriangleShape, CircleShape];

export interface GeometricAvatarProps {
  username: string;
  size?: number;
  online?: boolean;
  /** Optional: show lock icon badge */
  showLock?: boolean;
  className?: string;
}

export function GeometricAvatar({
  username,
  size = 46,
  online,
  showLock = false,
  className = "",
}: GeometricAvatarProps) {
  const hue = hashHue(username || "?");
  const idx = shapeIndex(username || "?");
  const ShapeComponent = SHAPES[idx];

  const bgFrom = hsl(hue, 30, 12);
  const bgTo = hsl((hue + 30) % 360, 25, 8);

  const dotSize = Math.round(size * 0.22);
  const lockSize = Math.round(size * 0.3);

  return (
    <div
      className={`relative flex-shrink-0 ${className}`}
      style={{ width: size, height: size }}
      aria-label={`${username} avatarı`}
    >
      <div
        style={{
          width: size,
          height: size,
          borderRadius: "50%",
          background: `linear-gradient(135deg, ${bgFrom}, ${bgTo})`,
          overflow: "hidden",
          position: "relative",
        }}
      >
        <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
          <ShapeComponent hue={hue} size={size} />
        </svg>
      </div>

      {/* Online indicator */}
      {online !== undefined && (
        <span
          aria-label={online ? "Çevrimiçi" : "Çevrimdışı"}
          style={{
            position: "absolute",
            bottom: -1,
            right: -1,
            width: dotSize,
            height: dotSize,
            borderRadius: "50%",
            background: online ? "#4ade80" : "rgba(232,232,240,0.25)",
            border: "1.5px solid var(--bg, #0d0d14)",
          }}
        />
      )}

      {/* Lock badge */}
      {showLock && (
        <div
          aria-hidden="true"
          style={{
            position: "absolute",
            bottom: -1,
            right: -1,
            width: lockSize,
            height: lockSize,
            borderRadius: "50%",
            background: "#4ade80",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontSize: Math.round(lockSize * 0.55),
            border: "1.5px solid var(--bg, #0d0d14)",
          }}
        >
          🔒
        </div>
      )}
    </div>
  );
}
