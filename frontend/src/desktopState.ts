import type {
  Agent,
  CustomResourceInput,
  InstallOperation,
  InstallPlan,
  ResourceCategory,
  ResourceIssue,
  ResourcePreview,
  Snapshot,
} from '../bindings/github.com/qinqingxu/acsync/internal/desktop/models'

export type AppAgent = Omit<Agent, 'exclude' | 'resources'> & {
  exclude: string[]
  resources: ResourceCategory[]
}

export type AppPreview = Omit<ResourcePreview, 'resources' | 'issues'> & {
  resources: ResourceCategory[]
  issues: ResourceIssue[]
}

export type AppInstallOperation = Omit<InstallOperation, 'args'> & {
  args: string[]
}

export type AppInstallPlan = Omit<InstallPlan, 'operations'> & {
  operations: AppInstallOperation[]
}

export type AppSnapshot = Omit<
  Snapshot,
  'agents' | 'preview' | 'conflicts' | 'customResources' | 'pendingInstallPlan'
> & {
  agents: AppAgent[]
  preview: AppPreview
  conflicts: NonNullable<Snapshot['conflicts']>
  customResources: CustomResourceInput[]
  pendingInstallPlan?: AppInstallPlan | null
}

export function normalizePreview(preview: ResourcePreview): AppPreview {
  return {
    ...preview,
    resources: preview.resources ?? [],
    issues: preview.issues ?? [],
  }
}

export function normalizeSnapshot(snapshot: Snapshot): AppSnapshot {
  return {
    ...snapshot,
    agents: (snapshot.agents ?? []).map((agent) => ({
      ...agent,
      exclude: agent.exclude ?? [],
      resources: agent.resources ?? [],
    })),
    preview: normalizePreview(snapshot.preview),
    conflicts: snapshot.conflicts ?? [],
    customResources: snapshot.customResources ?? [],
    pendingInstallPlan: snapshot.pendingInstallPlan
      ? {
          ...snapshot.pendingInstallPlan,
          operations: (snapshot.pendingInstallPlan.operations ?? []).map((operation) => ({
            ...operation,
            args: operation.args ?? [],
          })),
        }
      : null,
  }
}

export function hasGeneratedPreview(value: string): boolean {
  const generatedAt = new Date(value)
  return Boolean(value)
    && !Number.isNaN(generatedAt.getTime())
    && generatedAt.getUTCFullYear() > 1
}
