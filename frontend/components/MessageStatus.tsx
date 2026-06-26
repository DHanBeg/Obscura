"use client";

export type MessageStatusType = "sent" | "delivered" | "seen";

export function MessageStatusIcon({
  status,
  size = 14,
}: {
  status: MessageStatusType;
  size?: number;
}) {
  if (status === "seen") {
    // Double check — blue/green tint
    return (
      <svg
        width={size * 1.5}
        height={size}
        viewBox="0 0 24 16"
        fill="none"
        aria-label="Görüldü"
        role="img"
      >
        {/* First check */}
        <polyline
          points="1,8 5,12 11,4"
          stroke="rgba(74,222,128,0.9)"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {/* Second check */}
        <polyline
          points="7,8 11,12 17,4"
          stroke="rgba(74,222,128,0.9)"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    );
  }

  if (status === "delivered") {
    // Double check — grey
    return (
      <svg
        width={size * 1.5}
        height={size}
        viewBox="0 0 24 16"
        fill="none"
        aria-label="İletildi"
        role="img"
      >
        <polyline
          points="1,8 5,12 11,4"
          stroke="rgba(232,232,240,0.5)"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <polyline
          points="7,8 11,12 17,4"
          stroke="rgba(232,232,240,0.5)"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    );
  }

  // "sent" — single check grey
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      aria-label="Gönderildi"
      role="img"
    >
      <polyline
        points="2,8 6,12 14,4"
        stroke="rgba(232,232,240,0.35)"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

/** Map backend status string → MessageStatusType */
export function toStatusType(status: string): MessageStatusType {
  if (status === "read") return "seen";
  if (status === "delivered") return "delivered";
  return "sent";
}
