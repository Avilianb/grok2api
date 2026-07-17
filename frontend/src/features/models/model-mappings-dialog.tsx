import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  createModelMapping,
  deleteModelMapping,
  listModelMappings,
  updateModelMapping,
  type ModelMappingDTO,
  type ModelMappingInput,
  type ModelMappingTargetInput,
} from "@/entities/model/model-api";
import type { ModelRouteDTO } from "@/entities/model/types";
import { EmptyState, ErrorState, TableLoadingRow } from "@/shared/components/data-state";

type DraftTarget = ModelMappingTargetInput;

type DraftMapping = {
  externalId: string;
  enabled: boolean;
  effortOverride: string;
  targets: DraftTarget[];
};

const emptyDraft = (): DraftMapping => ({
  externalId: "",
  enabled: true,
  effortOverride: "",
  targets: [{ provider: "grok_console", upstreamModel: "", priority: 1, enabled: true }],
});

function toDraft(mapping: ModelMappingDTO): DraftMapping {
  const targets = [...mapping.targets]
    .sort((a, b) => a.priority - b.priority || a.id.localeCompare(b.id))
    .map((target, index) => ({
      provider: target.provider,
      upstreamModel: target.upstreamModel,
      priority: index + 1,
      enabled: target.enabled,
    }));
  return {
    externalId: mapping.externalId,
    enabled: mapping.enabled,
    effortOverride: mapping.effortOverride || "",
    targets: targets.length > 0 ? targets : emptyDraft().targets,
  };
}

function providerLabel(t: (key: string) => string, provider: ModelRouteDTO["provider"]): string {
  if (provider === "grok_web") return t("models.providerGrokWeb");
  if (provider === "grok_console") return t("console.name");
  return t("models.providerGrokBuild");
}

type ModelMappingsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ModelMappingsDialog({ open, onOpenChange }: ModelMappingsDialogProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<ModelMappingDTO | "new" | null>(null);
  const [draft, setDraft] = useState<DraftMapping>(emptyDraft);
  const [deleting, setDeleting] = useState<ModelMappingDTO | null>(null);

  const mappingsQuery = useQuery({
    queryKey: ["model-mappings"],
    queryFn: listModelMappings,
    enabled: open,
  });

  useEffect(() => {
    if (!open) {
      setEditing(null);
      setDeleting(null);
      setDraft(emptyDraft());
    }
  }, [open]);

  const saveMutation = useMutation({
    mutationFn: (input: ModelMappingInput) => {
      if (editing && editing !== "new") return updateModelMapping(editing.id, input);
      return createModelMapping(input);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["model-mappings"] });
      void queryClient.invalidateQueries({ queryKey: ["models"] });
      setEditing(null);
      toast.success(t(editing === "new" ? "models.mappingCreated" : "models.mappingUpdated"));
    },
    onError: showError,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteModelMapping(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["model-mappings"] });
      void queryClient.invalidateQueries({ queryKey: ["models"] });
      setDeleting(null);
      if (editing && editing !== "new" && deleting && editing.id === deleting.id) setEditing(null);
      toast.success(t("models.mappingDeleted"));
    },
    onError: showError,
  });

  function showError(error: unknown): void {
    toast.error(error instanceof Error ? error.message : t("errors.generic"));
  }

  function beginCreate(): void {
    setEditing("new");
    setDraft(emptyDraft());
  }

  function beginEdit(mapping: ModelMappingDTO): void {
    setEditing(mapping);
    setDraft(toDraft(mapping));
  }

  function updateTarget(index: number, patch: Partial<DraftTarget>): void {
    setDraft((current) => ({
      ...current,
      targets: current.targets.map((target, i) => (i === index ? { ...target, ...patch } : target)),
    }));
  }

  function moveTarget(index: number, direction: -1 | 1): void {
    setDraft((current) => {
      const nextIndex = index + direction;
      if (nextIndex < 0 || nextIndex >= current.targets.length) return current;
      const targets = [...current.targets];
      [targets[index], targets[nextIndex]] = [targets[nextIndex], targets[index]];
      return { ...current, targets: targets.map((target, i) => ({ ...target, priority: i + 1 })) };
    });
  }

  function removeTarget(index: number): void {
    setDraft((current) => {
      if (current.targets.length <= 1) return current;
      return {
        ...current,
        targets: current.targets.filter((_, i) => i !== index).map((target, i) => ({ ...target, priority: i + 1 })),
      };
    });
  }

  function addTarget(): void {
    setDraft((current) => ({
      ...current,
      targets: [
        ...current.targets,
        { provider: "grok_build", upstreamModel: "", priority: current.targets.length + 1, enabled: true },
      ],
    }));
  }

  function submitDraft(): void {
    const externalId = draft.externalId.trim();
    if (!externalId) {
      toast.error(t("errors.required"));
      return;
    }
    const targets = draft.targets.map((target, index) => ({
      provider: target.provider,
      upstreamModel: target.upstreamModel.trim(),
      priority: index + 1,
      enabled: target.enabled,
    }));
    if (targets.some((target) => !target.upstreamModel)) {
      toast.error(t("models.mappingUpstreamRequired"));
      return;
    }
    if (!targets.some((target) => target.enabled)) {
      toast.error(t("models.mappingEnableOne"));
      return;
    }
    saveMutation.mutate({ externalId, enabled: draft.enabled, effortOverride: draft.effortOverride, targets });
  }

  const items = mappingsQuery.data?.items ?? [];
  const formOpen = editing !== null;

  return (
    <>
      <Dialog open={open && !formOpen} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t("models.mappingTitle")}</DialogTitle>
            <DialogDescription>{t("models.mappingDescription")}</DialogDescription>
          </DialogHeader>
          <div className="flex justify-end">
            <Button size="sm" onClick={beginCreate}>{t("models.mappingCreate")}</Button>
          </div>
          {mappingsQuery.isError ? <ErrorState message={mappingsQuery.error.message} onRetry={() => void mappingsQuery.refetch()} /> : null}
          {mappingsQuery.isPending ? (
            <Table className="text-xs">
              <TableBody><TableLoadingRow colSpan={4} /></TableBody>
            </Table>
          ) : null}
          {!mappingsQuery.isPending && !mappingsQuery.isError && items.length === 0 ? <EmptyState /> : null}
          {!mappingsQuery.isPending && items.length > 0 ? (
            <Table className="text-xs">
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>{t("models.mappingExternalId")}</TableHead>
                  <TableHead>{t("models.mappingTargets")}</TableHead>
                  <TableHead className="text-center">{t("models.mappingEffort")}</TableHead>
                  <TableHead className="text-center">{t("models.status")}</TableHead>
                  <TableHead className="w-28 text-right">{t("common.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((mapping) => {
                  const ordered = [...mapping.targets].sort((a, b) => a.priority - b.priority || a.id.localeCompare(b.id));
                  return (
                    <TableRow key={mapping.id}>
                      <TableCell className="font-mono font-medium">{mapping.externalId}</TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          {ordered.map((target) => (
                            <Badge key={`${mapping.id}-${target.id}`} variant={target.enabled ? "secondary" : "outline"} className="font-mono text-[10px]">
                              {target.priority}. {providerLabel(t, target.provider)}/{target.upstreamModel}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell className="text-center font-mono text-xs text-muted-foreground">
                        {mapping.effortOverride || t("models.mappingEffortNone")}
                      </TableCell>
                      <TableCell className="text-center">
                        {mapping.enabled
                          ? <Badge variant="secondary" className="bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">{t("common.enabled")}</Badge>
                          : <Badge variant="outline" className="text-muted-foreground">{t("common.disabled")}</Badge>}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          <Button type="button" variant="ghost" size="sm" onClick={() => beginEdit(mapping)}>{t("common.edit")}</Button>
                          <Button type="button" variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={() => setDeleting(mapping)}>{t("common.delete")}</Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="secondary" size="sm" onClick={() => onOpenChange(false)}>{t("common.close")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={formOpen} onOpenChange={(next) => { if (!next) setEditing(null); }}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t(editing === "new" ? "models.mappingCreateTitle" : "models.mappingEditTitle")}</DialogTitle>
            <DialogDescription>{t("models.mappingFormDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="mapping-external-id">{t("models.mappingExternalId")}</Label>
              <Input id="mapping-external-id" className="font-mono" value={draft.externalId} onChange={(event) => setDraft((current) => ({ ...current, externalId: event.target.value }))} placeholder="claude-fable-5" />
            </div>
            <div className="flex items-center justify-between border-b py-2">
              <Label htmlFor="mapping-enabled">{draft.enabled ? t("common.enabled") : t("common.disabled")}</Label>
              <Switch id="mapping-enabled" checked={draft.enabled} onCheckedChange={(checked) => setDraft((current) => ({ ...current, enabled: checked }))} />
            </div>
            <div className="space-y-2">
              <Label>{t("models.mappingEffort")}</Label>
              <Select value={draft.effortOverride || "none"} onValueChange={(value) => setDraft((current) => ({ ...current, effortOverride: value === "none" ? "" : value }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">{t("models.mappingEffortNone")}</SelectItem>
                  <SelectItem value="low">low</SelectItem>
                  <SelectItem value="medium">medium</SelectItem>
                  <SelectItem value="high">high</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t("models.mappingEffortHint")}</p>
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>{t("models.mappingTargets")}</Label>
                <Button type="button" variant="secondary" size="sm" onClick={addTarget}><Plus className="size-3.5" />{t("models.mappingAddTarget")}</Button>
              </div>
              <p className="text-xs text-muted-foreground">{t("models.mappingPriorityHint")}</p>
              <div className="space-y-2">
                {draft.targets.map((target, index) => (
                  <div key={index} className="grid gap-2 rounded-md border p-3 sm:grid-cols-[auto_1fr_1fr_auto_auto]">
                    <div className="flex items-center gap-1">
                      <span className="w-5 text-center text-xs text-muted-foreground">{index + 1}</span>
                      <Button type="button" variant="ghost" size="icon" className="size-7" disabled={index === 0} onClick={() => moveTarget(index, -1)} aria-label={t("models.mappingMoveUp")}><ArrowUp className="size-3.5" /></Button>
                      <Button type="button" variant="ghost" size="icon" className="size-7" disabled={index === draft.targets.length - 1} onClick={() => moveTarget(index, 1)} aria-label={t("models.mappingMoveDown")}><ArrowDown className="size-3.5" /></Button>
                    </div>
                    <Select value={target.provider} onValueChange={(value) => updateTarget(index, { provider: value as ModelRouteDTO["provider"] })}>
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="grok_build">{t("models.providerGrokBuild")}</SelectItem>
                        <SelectItem value="grok_web">{t("models.providerGrokWeb")}</SelectItem>
                        <SelectItem value="grok_console">{t("console.name")}</SelectItem>
                      </SelectContent>
                    </Select>
                    <Input className="font-mono" value={target.upstreamModel} onChange={(event) => updateTarget(index, { upstreamModel: event.target.value })} placeholder="grok-4.3" />
                    <div className="flex items-center justify-between gap-2 sm:justify-center">
                      <Label className="text-xs text-muted-foreground sm:sr-only">{t("common.enabled")}</Label>
                      <Switch checked={target.enabled} onCheckedChange={(checked) => updateTarget(index, { enabled: checked })} />
                    </div>
                    <Button type="button" variant="ghost" size="icon" className="size-8 text-destructive hover:text-destructive" disabled={draft.targets.length <= 1} onClick={() => removeTarget(index)} aria-label={t("common.delete")}><Trash2 className="size-3.5" /></Button>
                  </div>
                ))}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="secondary" size="sm" onClick={() => setEditing(null)}>{t("common.cancel")}</Button>
            <Button type="button" size="sm" disabled={saveMutation.isPending} onClick={submitDraft}>
              {saveMutation.isPending ? <Spinner /> : null}
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={Boolean(deleting)} onOpenChange={(next) => !next && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("models.mappingDeleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("models.mappingDeleteDescription", { name: deleting?.externalId ?? "" })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction className="bg-destructive text-white hover:bg-destructive/90" disabled={deleteMutation.isPending} onClick={() => deleting && deleteMutation.mutate(deleting.id)}>
              {deleteMutation.isPending ? <Spinner /> : null}
              {t("common.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
