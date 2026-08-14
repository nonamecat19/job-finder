import type { CustomConnection, Resume, SocialNetwork } from '@job-finder/shared';
import { Field, Input, Button } from '../../../components/ui';

interface IdentityFormProps {
  resume: Resume;
  onChange: (resume: Resume) => void;
}

export function IdentityForm({ resume, onChange }: IdentityFormProps) {
  const set = <K extends keyof Resume>(key: K, value: Resume[K]) => onChange({ ...resume, [key]: value });

  const updateSocial = (i: number, patch: Partial<SocialNetwork>) => {
    const next = [...(resume.socialNetworks ?? [])];
    next[i] = { ...next[i], ...patch };
    set('socialNetworks', next);
  };
  const removeSocial = (i: number) => {
    set(
      'socialNetworks',
      (resume.socialNetworks ?? []).filter((_, idx) => idx !== i),
    );
  };
  const addSocial = () => set('socialNetworks', [...(resume.socialNetworks ?? []), { network: '', username: '' }]);

  const updateConnection = (i: number, patch: Partial<CustomConnection>) => {
    const next = [...(resume.customConnections ?? [])];
    next[i] = { ...next[i], ...patch };
    set('customConnections', next);
  };
  const removeConnection = (i: number) => {
    set(
      'customConnections',
      (resume.customConnections ?? []).filter((_, idx) => idx !== i),
    );
  };
  const addConnection = () =>
    set('customConnections', [...(resume.customConnections ?? []), { placeholder: '', url: '' }]);

  return (
    <div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label="Name">
          <Input value={resume.name} onChange={(e) => set('name', e.target.value)} placeholder="Jane Doe" />
        </Field>
        <Field label="Headline">
          <Input value={resume.headline ?? ''} onChange={(e) => set('headline', e.target.value || undefined)} />
        </Field>
        <Field label="Location">
          <Input value={resume.location ?? ''} onChange={(e) => set('location', e.target.value || undefined)} />
        </Field>
        <Field label="Email">
          <Input value={resume.email ?? ''} onChange={(e) => set('email', e.target.value || undefined)} />
        </Field>
        <Field label="Phone">
          <Input value={resume.phone ?? ''} onChange={(e) => set('phone', e.target.value || undefined)} />
        </Field>
        <Field label="Website">
          <Input value={resume.website ?? ''} onChange={(e) => set('website', e.target.value || undefined)} />
        </Field>
      </div>

      <div className="mt-4">
        <div className="flex items-center justify-between">
          <span className="font-mono text-xs font-medium tracking-[0.06em] text-muted uppercase">Social networks</span>
          <Button variant="ghost" onClick={addSocial}>
            + add
          </Button>
        </div>
        <div className="mt-2 space-y-2">
          {(resume.socialNetworks ?? []).map((sn, i) => (
            <div key={i} className="flex gap-2">
              <Input
                value={sn.network}
                onChange={(e) => updateSocial(i, { network: e.target.value })}
                placeholder="LinkedIn"
                className="w-1/3"
              />
              <Input
                value={sn.username}
                onChange={(e) => updateSocial(i, { username: e.target.value })}
                placeholder="username"
              />
              <Button variant="ghost" onClick={() => removeSocial(i)}>
                remove
              </Button>
            </div>
          ))}
        </div>
      </div>

      <div className="mt-4">
        <div className="flex items-center justify-between">
          <span className="font-mono text-xs font-medium tracking-[0.06em] text-muted uppercase">Custom connections</span>
          <Button variant="ghost" onClick={addConnection}>
            + add
          </Button>
        </div>
        <div className="mt-2 space-y-2">
          {(resume.customConnections ?? []).map((cc, i) => (
            <div key={i} className="flex gap-2">
              <Input
                value={cc.placeholder}
                onChange={(e) => updateConnection(i, { placeholder: e.target.value })}
                placeholder="Telegram"
                className="w-1/3"
              />
              <Input value={cc.url} onChange={(e) => updateConnection(i, { url: e.target.value })} placeholder="https://t.me/…" />
              <Input
                value={cc.icon ?? ''}
                onChange={(e) => updateConnection(i, { icon: e.target.value || undefined })}
                placeholder="icon (optional)"
                className="w-1/4"
              />
              <Button variant="ghost" onClick={() => removeConnection(i)}>
                remove
              </Button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
