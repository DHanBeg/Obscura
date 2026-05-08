"use client";
import { cn } from "@/lib/cn";

interface LogoProps {
  size?: number;
  className?: string;
  animated?: boolean;
}

export function ObscuraLogo({ size = 40, className, animated }: LogoProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 400 500"
      width={size}
      height={size * 1.25}
      className={cn(animated && "animate-float", className)}
    >
      <defs>
        <clipPath id="ec">
          <path d="M 192,48 L 228,76 L 264,62 L 285,102 L 277,150 L 260,183 L 234,208 L 214,235 L 226,272 L 248,312 L 265,354 L 272,388 L 250,370 L 224,344 L 204,368 L 192,394 L 174,374 L 175,344 L 155,361 L 143,388 L 123,368 L 127,335 L 106,352 L 93,376 L 78,356 L 84,319 L 79,289 L 75,260 L 65,242 L 52,225 L 68,208 L 93,197 L 116,182 L 136,163 L 149,140 L 160,115 L 169,89 L 177,67 Z" />
        </clipPath>
        <filter id="glow">
          <feGaussianBlur stdDeviation="4" result="coloredBlur" />
          <feMerge>
            <feMergeNode in="coloredBlur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <path
        d="M 192,48 L 228,76 L 264,62 L 285,102 L 277,150 L 260,183 L 234,208 L 214,235 L 226,272 L 248,312 L 265,354 L 272,388 L 250,370 L 224,344 L 204,368 L 192,394 L 174,374 L 175,344 L 155,361 L 143,388 L 123,368 L 127,335 L 106,352 L 93,376 L 78,356 L 84,319 L 79,289 L 75,260 L 65,242 L 52,225 L 68,208 L 93,197 L 116,182 L 136,163 L 149,140 L 160,115 L 169,89 L 177,67 Z"
        fill="#257830"
        filter="url(#glow)"
      />
      <g clipPath="url(#ec)">
        <g transform="rotate(-38, 175, 370)">
          <rect x="-60" y="238" width="480" height="20" fill="#0b2210" />
          <rect x="-60" y="268" width="480" height="20" fill="#0b2210" />
          <rect x="-60" y="298" width="480" height="20" fill="#0b2210" />
          <rect x="-60" y="328" width="480" height="20" fill="#0b2210" />
        </g>
      </g>
      <polygon points="168,106 202,120 186,150 153,136" fill="#5ec46e" />
    </svg>
  );
}

export function ObscuraWordmark({ className }: { className?: string }) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <ObscuraLogo size={32} />
      <span className="text-xl font-semibold tracking-tight text-head">
        obscura
      </span>
    </div>
  );
}
