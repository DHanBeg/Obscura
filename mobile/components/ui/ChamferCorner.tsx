import React from "react";
import { View, ViewStyle } from "react-native";

type Corner = "topLeft" | "topRight" | "bottomLeft" | "bottomRight";

const CORNER_POSITION: Record<Corner, ViewStyle> = {
  topLeft: { top: 0, left: 0 },
  topRight: { top: 0, right: 0 },
  bottomLeft: { bottom: 0, left: 0 },
  bottomRight: { bottom: 0, right: 0 },
};

// Only the horizontal edge (top/bottom) is painted; the vertical edge
// (left/right) is transparent but still needs its width set — that width is
// what miters the painted edge's far end into the diagonal hypotenuse.
// (Painting both edges the same color would fill the whole corner square
// instead of cutting a triangular notch.)
function cornerBorders(corner: Corner, size: number, color: string): ViewStyle {
  const base: ViewStyle = {
    borderTopWidth: 0, borderRightWidth: 0, borderBottomWidth: 0, borderLeftWidth: 0,
    borderTopColor: "transparent", borderRightColor: "transparent",
    borderBottomColor: "transparent", borderLeftColor: "transparent",
  };
  switch (corner) {
    case "topLeft":
      return { ...base, borderTopWidth: size, borderLeftWidth: size, borderTopColor: color };
    case "topRight":
      return { ...base, borderTopWidth: size, borderRightWidth: size, borderTopColor: color };
    case "bottomLeft":
      return { ...base, borderBottomWidth: size, borderLeftWidth: size, borderBottomColor: color };
    case "bottomRight":
      return { ...base, borderBottomWidth: size, borderRightWidth: size, borderBottomColor: color };
  }
}

interface Props {
  corner: Corner;
  size?: number;
  color: string;
}

/**
 * Faceted "chamfer" corner illusion. React Native has no clip-path, so this
 * fakes the cut-corner look with a 0x0 box whose two adjacent borders (same
 * color) render as a right triangle, masking the sharp corner underneath
 * with whatever color sits directly behind the card.
 */
export function ChamferCorner({ corner, size = 20, color }: Props) {
  return (
    <View
      pointerEvents="none"
      style={[
        { position: "absolute", width: 0, height: 0 },
        CORNER_POSITION[corner],
        cornerBorders(corner, size, color),
      ]}
    />
  );
}
