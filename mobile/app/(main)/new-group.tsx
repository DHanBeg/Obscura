import React, { useState, useCallback, useRef, useEffect } from "react";
import {
  View, Text, TextInput, TouchableOpacity, StyleSheet,
  FlatList, ActivityIndicator, ScrollView,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { colors, spacing, radius, typography } from "@/lib/theme";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { displayIdentifier } from "@/lib/odi-display";
import { createMlsGroupConversation } from "@/lib/mls/createGroupFlow";

interface UserResult {
  did: string;
  odi?: string;
  display_name?: string;
  username?: string;
}

function StepDots({ current, total }: { current: number; total: number }) {
  return (
    <View style={styles.stepDots}>
      {Array.from({ length: total }).map((_, i) => (
        <View
          key={i}
          style={[styles.dot, i === current && styles.dotActive, { width: i === current ? 20 : 7 }]}
        />
      ))}
    </View>
  );
}

export default function NewGroupScreen() {
  const { user } = useStore();
  const [step, setStep] = useState(0);
  const [groupName, setGroupName] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<UserResult[]>([]);
  const [selectedMembers, setSelectedMembers] = useState<UserResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSearch = useCallback((q: string) => {
    setSearchQuery(q);
    if (searchTimer.current) clearTimeout(searchTimer.current);
    if (!q.trim()) { setSearchResults([]); setIsSearching(false); return; }
    setIsSearching(true);
    searchTimer.current = setTimeout(async () => {
      try {
        const res = await api.searchUsers(q);
        setSearchResults(res?.users || []);
      } catch {}
      finally { setIsSearching(false); }
    }, 300);
  }, []);

  useEffect(() => () => {
    if (searchTimer.current) clearTimeout(searchTimer.current);
  }, []);

  const toggleMember = (user: UserResult) => {
    setSelectedMembers((prev) =>
      prev.find((m) => m.did === user.did)
        ? prev.filter((m) => m.did !== user.did)
        : [...prev, user]
    );
  };

  const displayName = (u: UserResult) => u.display_name || u.username || displayIdentifier(u);

  // Tuğla 5b-2 Parça D — grup ARTIK MLS ile kurulur (createGroupFlow.ts,
  // sıralı: local ts-mls state → local kayıt → backend MLS kaydı+add → conv
  // link). Eski düz-plaintext grup oluşturma çağrısı (doğrudan createConversation,
  // grup tipiyle) BİLEREK kaldırıldı (Ders-4: plaintext-capable bir yol var olmamalı) —
  // MÜHÜR gereği bir başarısızlıkta buraya DÜŞÜLMEZ, hata olduğu gibi
  // kullanıcıya gösterilir.
  const submit = async () => {
    if (!groupName.trim()) { setError("Grup adı zorunludur"); return; }
    if (!user) { setError("Oturum bulunamadı"); return; }
    setError(null);
    setIsSubmitting(true);
    try {
      const result = await createMlsGroupConversation({
        ownDid: user.did,
        name: groupName,
        memberDids: selectedMembers.map((m) => m.did),
      });
      router.replace(`/(main)/chat/${result.convId}`);
    } catch {
      setError("Grup oluşturulamadı");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <SafeAreaView style={styles.root} edges={["top"]}>
      {/* Header */}
      <View style={styles.header}>
        <TouchableOpacity onPress={() => (step === 0 ? router.back() : setStep(step - 1))} style={styles.backBtn}>
          <Ionicons name="chevron-back" size={24} color={colors.head} />
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Grup Oluştur</Text>
        <View style={{ width: 40 }} />
      </View>

      <StepDots current={step} total={2} />

      {step === 0 && (
        <ScrollView contentContainerStyle={styles.stepContent}>
          <View style={styles.iconWrap}>
            <Ionicons name="people-outline" size={32} color={colors.accent} />
          </View>
          <Text style={styles.stepTitle}>Grup Adı</Text>
          <Text style={styles.stepSub}>Grubunuz nasıl adlandırılsın?</Text>
          <TextInput
            style={styles.nameInput}
            value={groupName}
            onChangeText={setGroupName}
            placeholder="Grup adı..."
            placeholderTextColor={colors.dim}
            autoFocus
            maxLength={64}
          />
          <Text style={styles.charCount}>{groupName.length}/64</Text>
          {error && <Text style={styles.errorText}>{error}</Text>}
          <TouchableOpacity
            style={[styles.nextBtn, !groupName.trim() && styles.nextBtnDim]}
            onPress={() => { if (groupName.trim()) setStep(1); else setError("Grup adı zorunludur"); }}
          >
            <Text style={styles.nextBtnText}>Devam</Text>
            <Ionicons name="chevron-forward" size={18} color={colors.void} />
          </TouchableOpacity>
        </ScrollView>
      )}

      {step === 1 && (
        <View style={{ flex: 1 }}>
          {/* Selected chips */}
          {selectedMembers.length > 0 && (
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.chips}>
              {selectedMembers.map((m) => (
                <TouchableOpacity key={m.did} style={styles.chip} onPress={() => toggleMember(m)}>
                  <Text style={styles.chipText}>{displayName(m)}</Text>
                  <Ionicons name="close" size={13} color={colors.sub} />
                </TouchableOpacity>
              ))}
            </ScrollView>
          )}

          {/* Search */}
          <View style={styles.searchBox}>
            <Ionicons name="search-outline" size={16} color={colors.sub} />
            <TextInput
              style={styles.searchInput}
              value={searchQuery}
              onChangeText={handleSearch}
              placeholder="Kişi ara..."
              placeholderTextColor={colors.dim}
              autoCapitalize="none"
            />
            {isSearching && <ActivityIndicator size="small" color={colors.sub} />}
          </View>

          <FlatList
            data={searchResults}
            keyExtractor={(u) => u.did}
            renderItem={({ item }) => {
              const selected = !!selectedMembers.find((m) => m.did === item.did);
              return (
                <TouchableOpacity style={styles.userRow} onPress={() => toggleMember(item)}>
                  <View style={styles.avatar}>
                    <Text style={styles.avatarText}>{displayName(item)[0]?.toUpperCase()}</Text>
                  </View>
                  <Text style={styles.userName}>{displayName(item)}</Text>
                  {selected && <Ionicons name="checkmark-circle" size={20} color={colors.accent} />}
                </TouchableOpacity>
              );
            }}
            contentContainerStyle={{ paddingHorizontal: spacing.md }}
            ListEmptyComponent={
              searchQuery.length > 0 && !isSearching ? (
                <Text style={styles.noResult}>Sonuç bulunamadı</Text>
              ) : null
            }
          />

          {error && <Text style={[styles.errorText, { paddingHorizontal: spacing.lg }]}>{error}</Text>}

          <View style={styles.submitWrap}>
            <TouchableOpacity
              style={[styles.nextBtn, (isSubmitting || selectedMembers.length === 0) && styles.nextBtnDim]}
              onPress={submit}
              disabled={isSubmitting || selectedMembers.length === 0}
            >
              {isSubmitting
                ? <ActivityIndicator size="small" color={colors.void} />
                : <>
                  <Text style={styles.nextBtnText}>Grubu Oluştur ({selectedMembers.length})</Text>
                  <Ionicons name="checkmark" size={18} color={colors.void} />
                </>}
            </TouchableOpacity>
          </View>
        </View>
      )}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.void },
  header: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: spacing.lg, paddingVertical: spacing.md,
    borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  backBtn: { padding: 4, width: 40 },
  headerTitle: { fontSize: typography.base, fontWeight: "700", color: colors.head },
  stepDots: { flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 6, paddingVertical: 16 },
  dot: { height: 7, borderRadius: 99, backgroundColor: colors.muted },
  dotActive: { backgroundColor: colors.accent },
  stepContent: { padding: spacing.xl, alignItems: "center", gap: spacing.md },
  iconWrap: {
    width: 80, height: 80, borderRadius: 24,
    backgroundColor: "rgba(74,222,128,0.1)", alignItems: "center", justifyContent: "center",
    marginBottom: spacing.sm,
  },
  stepTitle: { fontSize: typography.xl, fontWeight: "700", color: colors.head },
  stepSub: { fontSize: typography.sm, color: colors.sub, textAlign: "center" },
  nameInput: {
    width: "100%", backgroundColor: colors.surface, borderRadius: radius.xl,
    borderWidth: 1, borderColor: colors.border, padding: spacing.md,
    fontSize: typography.md, color: colors.head, textAlign: "center",
  },
  charCount: { fontSize: typography.xs, color: colors.dim },
  errorText: { fontSize: typography.xs, color: colors.red },
  nextBtn: {
    flexDirection: "row", alignItems: "center", gap: spacing.xs,
    paddingHorizontal: spacing.xl, paddingVertical: 14,
    backgroundColor: colors.accent, borderRadius: radius.xl,
  },
  nextBtnDim: { opacity: 0.4 },
  nextBtnText: { fontSize: typography.base, fontWeight: "700", color: colors.void },
  chips: { paddingHorizontal: spacing.md, paddingVertical: spacing.sm, gap: spacing.xs },
  chip: {
    flexDirection: "row", alignItems: "center", gap: 6,
    paddingHorizontal: spacing.sm, paddingVertical: 6,
    backgroundColor: "rgba(74,222,128,0.1)", borderRadius: radius.full,
    borderWidth: 1, borderColor: "rgba(74,222,128,0.2)",
  },
  chipText: { fontSize: typography.xs, color: colors.head },
  searchBox: {
    flexDirection: "row", alignItems: "center", gap: spacing.sm,
    margin: spacing.md, padding: spacing.sm, paddingHorizontal: spacing.md,
    backgroundColor: colors.surface, borderRadius: radius.xl, borderWidth: 1, borderColor: colors.border,
  },
  searchInput: { flex: 1, fontSize: typography.sm, color: colors.head },
  userRow: {
    flexDirection: "row", alignItems: "center", gap: spacing.md,
    paddingVertical: 12, borderBottomWidth: 1, borderBottomColor: colors.border,
  },
  avatar: {
    width: 40, height: 40, borderRadius: 20,
    backgroundColor: colors.raised, alignItems: "center", justifyContent: "center",
  },
  avatarText: { fontSize: typography.base, fontWeight: "600", color: colors.body },
  userName: { flex: 1, fontSize: typography.base, color: colors.head },
  noResult: { textAlign: "center", padding: 24, color: colors.sub, fontSize: typography.sm },
  submitWrap: { padding: spacing.lg },
});
