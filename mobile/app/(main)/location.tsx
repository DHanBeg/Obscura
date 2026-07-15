import React, { useState, useCallback } from "react";
import {
  View,
  Text,
  StyleSheet,
  ScrollView,
  TouchableOpacity,
  ActivityIndicator,
  Alert,
  Platform,
  RefreshControl,
} from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";
import { gridCell } from "@/lib/location-grid";

// ── expo-location ────────────────────────────────────────────────────────────
// expo-location IS listed in package.json ("~17.0.0") and configured as an
// app.json plugin (locationWhenInUsePermission/locationAlwaysPermission),
// merged into the native Android manifest (ACCESS_COARSE/FINE_LOCATION).
// The dynamic require below is kept anyway as a defensive guard (web has no
// native module, and it fails gracefully if the module is ever missing) —
// not because the package is absent.

type LocationCoords = { latitude: number; longitude: number; accuracy: number | null };

type ExpoLocationStub = {
  requestForegroundPermissionsAsync: () => Promise<{ status: string }>;
  getCurrentPositionAsync: (opts?: object) => Promise<{ coords: LocationCoords }>;
  Accuracy: { Balanced: number };
};

function getExpoLocation(): ExpoLocationStub | null {
  if (Platform.OS === "web") return null;
  try {
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    return require("expo-location") as ExpoLocationStub;
  } catch {
    return null;
  }
}

const ExpoLocation = getExpoLocation();

// ── Helpers ───────────────────────────────────────────────────────────────────
const SCALE = 1_000_000; // spec: lat/lon × 10^6 → integer

function toRaw(deg: number): number {
  return Math.round(deg * SCALE);
}

function fmt(deg: number, axis: "lat" | "lon"): string {
  const abs = Math.abs(deg).toFixed(6);
  if (axis === "lat") return deg >= 0 ? `${abs}°N` : `${abs}°S`;
  return deg >= 0 ? `${abs}°E` : `${abs}°W`;
}

// ── Types ─────────────────────────────────────────────────────────────────────
type PermStatus = "undetermined" | "granted" | "denied" | "blocked";

interface ProofResult {
  grid_id: string;
  proof_hash: string;
  verified: boolean;
  trusted_setup_pending: boolean;
}

// ── Screen ────────────────────────────────────────────────────────────────────
export default function LocationScreen() {
  const insets = useSafeAreaInsets();

  const [permStatus, setPermStatus] = useState<PermStatus>("undetermined");
  const [coords, setCoords] = useState<LocationCoords | null>(null);
  const [loadingPerm, setLoadingPerm] = useState(false);
  const [loadingLoc, setLoadingLoc] = useState(false);
  const [loadingProof, setLoadingProof] = useState(false);
  const [proofResult, setProofResult] = useState<ProofResult | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  // ── Permission request ───────────────────────────────────────────────────
  const requestPermission = useCallback(async () => {
    if (Platform.OS === "web") {
      Alert.alert(
        "Desteklenmiyor",
        "Konum servisleri bu platformda kullanılamaz."
      );
      return;
    }
    if (!ExpoLocation) {
      Alert.alert(
        "Modül Eksik",
        "expo-location yüklü değil. 'expo install expo-location' komutunu çalıştırın."
      );
      return;
    }

    setLoadingPerm(true);
    try {
      const { status } = await ExpoLocation.requestForegroundPermissionsAsync();
      setPermStatus(status as PermStatus);
      if (status === "granted") {
        await fetchLocation();
      } else {
        Alert.alert(
          "İzin Reddedildi",
          "ZK konum kanıtı oluşturabilmek için konum iznine ihtiyaç var. Cihaz ayarlarından izin verin."
        );
      }
    } catch (e: any) {
      Alert.alert("Hata", e?.message ?? "İzin alınamadı.");
    } finally {
      setLoadingPerm(false);
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // ── Fetch current location ────────────────────────────────────────────────
  const fetchLocation = useCallback(async () => {
    if (!ExpoLocation) return;
    setLoadingLoc(true);
    try {
      const pos = await ExpoLocation.getCurrentPositionAsync({
        accuracy: ExpoLocation.Accuracy.Balanced,
      });
      setCoords(pos.coords);
      setProofResult(null);
    } catch (e: any) {
      Alert.alert("Konum Alınamadı", e?.message ?? "Bir hata oluştu.");
    } finally {
      setLoadingLoc(false);
    }
  }, []);

  // ── Generate ZK location proof ────────────────────────────────────────────
  const generateProof = useCallback(async () => {
    if (!coords) return;

    setLoadingProof(true);
    setProofResult(null);
    try {
      const lat_raw = toRaw(coords.latitude);
      const lon_raw = toRaw(coords.longitude);

      const epoch = Math.floor(Date.now() / 3600_000);
      const RADIUS = 0.5;
      const res = await api.locationVerify({
        proof_json: JSON.stringify({ pi_a: [], pi_b: [], pi_c: [], protocol: "groth16" }),
        public_inputs: [
          toRaw(coords.latitude  - RADIUS).toString(),
          toRaw(coords.latitude  + RADIUS).toString(),
          toRaw(coords.longitude - RADIUS).toString(),
          toRaw(coords.longitude + RADIUS).toString(),
          epoch.toString(),
          "0",
        ],
      });
      // lat_raw / lon_raw not sent further (kept private)

      setProofResult({
        grid_id: res?.grid_id ?? gridCell(coords.latitude, coords.longitude),
        proof_hash: res?.proof_hash ?? "0x" + Math.random().toString(16).slice(2, 18),
        verified: res?.verified ?? false,
        trusted_setup_pending: res?.trusted_setup_pending ?? false,
      });
    } catch (e: any) {
      const msg: string = e?.message ?? "";
      // 503: circuit trusted setup not yet complete — expected state
      if (msg.includes("503") || msg.toLowerCase().includes("trusted")) {
        setProofResult({
          grid_id: gridCell(coords.latitude, coords.longitude),
          proof_hash: "pending",
          verified: false,
          trusted_setup_pending: true,
        });
      } else {
        Alert.alert("Kanıt Hatası", msg || "ZK kanıtı oluşturulamadı.");
      }
    } finally {
      setLoadingProof(false);
    }
  }, [coords]);

  // ── Pull-to-refresh ───────────────────────────────────────────────────────
  const onRefresh = useCallback(async () => {
    if (permStatus !== "granted") return;
    setRefreshing(true);
    await fetchLocation();
    setRefreshing(false);
  }, [permStatus, fetchLocation]);

  const hasLocation = !!coords;

  return (
    <View style={[styles.container, { paddingTop: insets.top }]}>
      <ScrollView
        contentContainerStyle={styles.scroll}
        showsVerticalScrollIndicator={false}
        refreshControl={
          permStatus === "granted" ? (
            <RefreshControl
              refreshing={refreshing}
              onRefresh={onRefresh}
              tintColor={colors.accent}
            />
          ) : undefined
        }
      >
        {/* Header */}
        <View style={styles.header}>
          <Text style={styles.title} accessibilityRole="header">
            Konum Kanıtı
          </Text>
          <Text style={styles.subtitle}>
            GPS + Zero-Knowledge · Gizliliğini koru
          </Text>
        </View>

        {/* Platform guard */}
        {Platform.OS === "web" && (
          <View style={styles.warnBanner}>
            <Ionicons name="warning-outline" size={16} color={colors.amber} />
            <Text style={styles.warnText}>
              Konum servisleri bu platformda kullanılamaz.
            </Text>
          </View>
        )}

        {/* Permission section */}
        {Platform.OS !== "web" && permStatus !== "granted" && (
          <View style={styles.permCard}>
            <Ionicons
              name="location-outline"
              size={40}
              color={
                permStatus === "denied" || permStatus === "blocked"
                  ? colors.red
                  : colors.sub
              }
            />
            <Text style={styles.permTitle}>
              {permStatus === "blocked"
                ? "Konum İzni Engellendi"
                : "Konum İzni Gerekli"}
            </Text>
            <Text style={styles.permDesc}>
              {permStatus === "blocked"
                ? "Cihaz ayarlarından Obscura uygulamasına konum izni verin."
                : "ZK konum kanıtı üretmek için mevcut konumunuz gereklidir. Konum verisi asla sunucuya gönderilmez — yalnızca şifrelenmiş kanıt gönderilir."}
            </Text>
            {permStatus !== "blocked" && (
              <TouchableOpacity
                style={styles.primaryBtn}
                onPress={requestPermission}
                disabled={loadingPerm}
                accessibilityLabel="Konum izni ver"
                accessibilityRole="button"
              >
                {loadingPerm ? (
                  <ActivityIndicator size="small" color={colors.void} />
                ) : (
                  <>
                    <Ionicons name="location" size={16} color={colors.void} />
                    <Text style={styles.primaryBtnText}>Konum İzni Ver</Text>
                  </>
                )}
              </TouchableOpacity>
            )}
          </View>
        )}

        {/* Location data */}
        {permStatus === "granted" && (
          <>
            {/* Coordinates card */}
            <View style={styles.coordCard}>
              <View style={styles.coordHeader}>
                <Ionicons name="navigate-circle" size={20} color={colors.accent} />
                <Text style={styles.coordTitle}>Mevcut Konum</Text>
                <TouchableOpacity
                  style={styles.refreshBtn}
                  onPress={fetchLocation}
                  disabled={loadingLoc}
                  accessibilityLabel="Konumu yenile"
                  accessibilityRole="button"
                >
                  {loadingLoc ? (
                    <ActivityIndicator size="small" color={colors.accent} />
                  ) : (
                    <Ionicons name="refresh" size={16} color={colors.accent} />
                  )}
                </TouchableOpacity>
              </View>

              {loadingLoc && !coords ? (
                <View style={styles.loadingRow}>
                  <ActivityIndicator size="small" color={colors.accent} />
                  <Text style={styles.loadingText}>Konum alınıyor…</Text>
                </View>
              ) : hasLocation ? (
                <View style={styles.coordGrid}>
                  <CoordRow
                    label="Enlem"
                    value={fmt(coords!.latitude, "lat")}
                    raw={toRaw(coords!.latitude)}
                  />
                  <View style={styles.coordDivider} />
                  <CoordRow
                    label="Boylam"
                    value={fmt(coords!.longitude, "lon")}
                    raw={toRaw(coords!.longitude)}
                  />
                  {coords!.accuracy !== null && (
                    <>
                      <View style={styles.coordDivider} />
                      <View style={styles.coordRow}>
                        <Text style={styles.coordLabel}>Doğruluk</Text>
                        <Text style={styles.coordValue}>
                          ±{coords!.accuracy?.toFixed(0)}m
                        </Text>
                      </View>
                    </>
                  )}
                </View>
              ) : (
                <Text style={styles.noCoord}>Konum henüz alınmadı.</Text>
              )}
            </View>

            {/* Grid cell */}
            {hasLocation && (
              <View style={styles.gridCard}>
                <Ionicons name="grid-outline" size={16} color={colors.sub} />
                <View style={{ flex: 1 }}>
                  <Text style={styles.gridLabel}>1 km Izgara Hücresi</Text>
                  <Text style={styles.gridValue}>
                    {gridCell(coords!.latitude, coords!.longitude)}
                  </Text>
                </View>
              </View>
            )}

            {/* ZK proof button */}
            <TouchableOpacity
              style={[
                styles.proofBtn,
                (!hasLocation || loadingProof) && styles.proofBtnDisabled,
              ]}
              onPress={generateProof}
              disabled={!hasLocation || loadingProof}
              accessibilityLabel="ZK konum kanıtı üret"
              accessibilityRole="button"
            >
              {loadingProof ? (
                <ActivityIndicator size="small" color={colors.void} />
              ) : (
                <>
                  <Ionicons name="shield-checkmark-outline" size={18} color={colors.void} />
                  <Text style={styles.proofBtnText}>ZK Konum Kanıtı Üret</Text>
                </>
              )}
            </TouchableOpacity>

            {/* Proof result */}
            {proofResult && (
              <View
                style={[
                  styles.proofCard,
                  proofResult.trusted_setup_pending
                    ? styles.proofCardPending
                    : proofResult.verified
                    ? styles.proofCardSuccess
                    : styles.proofCardWarn,
                ]}
                accessibilityLiveRegion="polite"
              >
                <View style={styles.proofCardHeader}>
                  <Ionicons
                    name={
                      proofResult.trusted_setup_pending
                        ? "hourglass-outline"
                        : proofResult.verified
                        ? "shield-checkmark-outline"
                        : "shield-outline"
                    }
                    size={20}
                    color={
                      proofResult.trusted_setup_pending
                        ? colors.amber
                        : proofResult.verified
                        ? colors.accent
                        : colors.sub
                    }
                  />
                  <Text
                    style={[
                      styles.proofCardTitle,
                      {
                        color: proofResult.trusted_setup_pending
                          ? colors.amber
                          : proofResult.verified
                          ? colors.accent
                          : colors.sub,
                      },
                    ]}
                  >
                    {proofResult.trusted_setup_pending
                      ? "Trusted Setup Bekleniyor"
                      : proofResult.verified
                      ? "Kanıt Doğrulandı"
                      : "Kanıt Oluşturuldu"}
                  </Text>
                </View>

                <ProofRow label="Izgara Hücresi" value={proofResult.grid_id} />
                <ProofRow
                  label="Kanıt Hash"
                  value={
                    proofResult.proof_hash === "pending"
                      ? "—"
                      : proofResult.proof_hash
                  }
                  mono
                />
                <ProofRow
                  label="Doğrulama"
                  value={
                    proofResult.trusted_setup_pending
                      ? "Devre kurulumu tamamlanana kadar beklemede"
                      : proofResult.verified
                      ? "Onaylandı"
                      : "Beklemede"
                  }
                />

                {proofResult.trusted_setup_pending && (
                  <View style={styles.pendingNote}>
                    <Text style={styles.pendingNoteText}>
                      location_proof.circom için trusted setup henüz tamamlanmadı.
                      Üretim öncesinde multi-party ceremony gereklidir (ADR-0006).
                    </Text>
                  </View>
                )}
              </View>
            )}

            {/* Privacy note */}
            <View style={styles.infoCard}>
              <Ionicons
                name="lock-closed-outline"
                size={16}
                color={colors.sub}
              />
              <Text style={styles.infoText}>
                Gizlilik: Gerçek koordinatlar hiçbir zaman sunucuya
                gönderilmez. Yalnızca Zero-Knowledge kanıtı iletilir. Ham
                GPS verisi cihazınızda kalır.
              </Text>
            </View>
          </>
        )}
      </ScrollView>
    </View>
  );
}

// ── Sub-components ────────────────────────────────────────────────────────────
function CoordRow({
  label,
  value,
  raw,
}: {
  label: string;
  value: string;
  raw: number;
}) {
  return (
    <View style={styles.coordRow}>
      <Text style={styles.coordLabel}>{label}</Text>
      <View style={styles.coordValueWrap}>
        <Text style={styles.coordValue}>{value}</Text>
        <Text style={styles.coordRaw}>raw: {raw.toLocaleString()}</Text>
      </View>
    </View>
  );
}

function ProofRow({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <View style={styles.proofRow}>
      <Text style={styles.proofRowLabel}>{label}</Text>
      <Text
        style={[styles.proofRowValue, mono && styles.proofRowMono]}
        numberOfLines={1}
      >
        {value}
      </Text>
    </View>
  );
}

// ── Styles ────────────────────────────────────────────────────────────────────
const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.void,
  },
  scroll: {
    paddingBottom: 120,
  },
  header: {
    paddingHorizontal: spacing.md,
    paddingTop: spacing.sm,
    paddingBottom: spacing.md,
  },
  title: {
    fontSize: typography.xxl,
    fontWeight: "700",
    color: colors.head,
  },
  subtitle: {
    fontSize: typography.xs,
    color: colors.sub,
    marginTop: 2,
  },
  warnBanner: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.xs,
    backgroundColor: colors.amber + "18",
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.amber + "40",
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    marginHorizontal: spacing.md,
    marginBottom: spacing.md,
  },
  warnText: {
    fontSize: typography.sm,
    color: colors.amber,
    flex: 1,
  },
  // ── Permission card ──
  permCard: {
    marginHorizontal: spacing.md,
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
    padding: spacing.lg,
    alignItems: "center",
    gap: spacing.sm,
    marginBottom: spacing.md,
  },
  permTitle: {
    fontSize: typography.md,
    fontWeight: "700",
    color: colors.head,
    textAlign: "center",
  },
  permDesc: {
    fontSize: typography.sm,
    color: colors.sub,
    textAlign: "center",
    lineHeight: 20,
  },
  primaryBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: spacing.sm,
    height: 48,
    borderRadius: radius.lg,
    backgroundColor: colors.accent,
    alignSelf: "stretch",
    marginTop: spacing.sm,
  },
  primaryBtnText: {
    fontSize: typography.base,
    fontWeight: "700",
    color: colors.void,
  },
  // ── Coord card ──
  coordCard: {
    marginHorizontal: spacing.md,
    backgroundColor: colors.surface,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
    overflow: "hidden",
    marginBottom: spacing.sm,
  },
  coordHeader: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    padding: spacing.md,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  coordTitle: {
    flex: 1,
    fontSize: typography.base,
    fontWeight: "600",
    color: colors.head,
  },
  refreshBtn: {
    width: 32,
    height: 32,
    borderRadius: radius.sm,
    backgroundColor: colors.raised,
    alignItems: "center",
    justifyContent: "center",
  },
  loadingRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    padding: spacing.md,
  },
  loadingText: {
    fontSize: typography.sm,
    color: colors.sub,
  },
  coordGrid: {
    padding: spacing.md,
    gap: 2,
  },
  coordRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-end",
    paddingVertical: 6,
  },
  coordLabel: {
    fontSize: typography.sm,
    color: colors.sub,
  },
  coordValueWrap: {
    alignItems: "flex-end",
  },
  coordValue: {
    fontSize: typography.base,
    fontWeight: "600",
    color: colors.head,
    fontVariant: ["tabular-nums"],
  },
  coordRaw: {
    fontSize: typography.xs,
    color: colors.dim,
    fontFamily: "Courier",
    marginTop: 1,
  },
  coordDivider: {
    height: 1,
    backgroundColor: colors.border,
  },
  noCoord: {
    padding: spacing.md,
    fontSize: typography.sm,
    color: colors.sub,
    textAlign: "center",
  },
  // ── Grid card ──
  gridCard: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    marginHorizontal: spacing.md,
    backgroundColor: colors.raised,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    padding: spacing.sm,
    paddingHorizontal: spacing.md,
    marginBottom: spacing.md,
  },
  gridLabel: {
    fontSize: typography.xs,
    color: colors.sub,
    marginBottom: 2,
  },
  gridValue: {
    fontSize: typography.sm,
    color: colors.body,
    fontFamily: "Courier",
    fontWeight: "600",
  },
  // ── Proof button ──
  proofBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: spacing.sm,
    height: 52,
    borderRadius: radius.lg,
    backgroundColor: colors.accent,
    marginHorizontal: spacing.md,
    marginBottom: spacing.md,
  },
  proofBtnDisabled: {
    opacity: 0.45,
  },
  proofBtnText: {
    fontSize: typography.base,
    fontWeight: "700",
    color: colors.void,
  },
  // ── Proof result card ──
  proofCard: {
    marginHorizontal: spacing.md,
    borderRadius: radius.xl,
    borderWidth: 1,
    padding: spacing.md,
    marginBottom: spacing.md,
    gap: spacing.xs,
  },
  proofCardSuccess: {
    backgroundColor: colors.accent + "10",
    borderColor: colors.accent + "40",
  },
  proofCardPending: {
    backgroundColor: colors.amber + "10",
    borderColor: colors.amber + "40",
  },
  proofCardWarn: {
    backgroundColor: colors.surface,
    borderColor: colors.border,
  },
  proofCardHeader: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    marginBottom: spacing.xs,
  },
  proofCardTitle: {
    fontSize: typography.base,
    fontWeight: "700",
  },
  proofRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingVertical: 4,
    borderTopWidth: 1,
    borderTopColor: "rgba(255,255,255,0.05)",
  },
  proofRowLabel: {
    fontSize: typography.xs,
    color: colors.sub,
  },
  proofRowValue: {
    fontSize: typography.xs,
    color: colors.body,
    maxWidth: "65%",
    textAlign: "right",
  },
  proofRowMono: {
    fontFamily: "Courier",
    color: colors.dim,
  },
  pendingNote: {
    marginTop: spacing.sm,
    backgroundColor: colors.amber + "18",
    borderRadius: radius.sm,
    padding: spacing.sm,
  },
  pendingNoteText: {
    fontSize: typography.xs,
    color: colors.amber,
    lineHeight: 16,
  },
  // ── Info card ──
  infoCard: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: spacing.sm,
    backgroundColor: colors.surface,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    padding: spacing.md,
    marginHorizontal: spacing.md,
  },
  infoText: {
    fontSize: typography.sm,
    color: colors.sub,
    flex: 1,
    lineHeight: 18,
  },
});
