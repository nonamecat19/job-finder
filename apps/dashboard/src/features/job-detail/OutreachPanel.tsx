import { ChevronDown, ChevronUp, Sparkles } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { OutreachDraftDto, OutreachTone } from '@job-finder/shared';
import { Button, Chip, ErrorState, Field, Select, Spinner, Textarea } from '../../components/ui';
import { emitToast, toErrorMessage } from '../../lib/toastBus';
import { useGenerateOutreachDraft, useJobContacts, useOutreachTones } from './hooks';

/**
 * OutreachPanel is the post-apply outreach draft generator (spec 012): pick
 * a contact and a tone, generate a short grounded draft, copy it out by
 * hand. This is the entire surface of the feature — the only actions on a
 * generated draft are edit, copy, and regenerate. There is no send,
 * schedule, or deliver action anywhere here, by design (FR-002, FR-003):
 * the system drafts, the user sends it themselves.
 */
export default function OutreachPanel({ jobId }: { jobId: string | undefined }) {
  const { data: contacts } = useJobContacts(jobId);
  const { data: tones } = useOutreachTones(jobId);
  const generate = useGenerateOutreachDraft(jobId);

  const [contactId, setContactId] = useState('');
  const [tone, setTone] = useState<OutreachTone | ''>('');
  const [draft, setDraft] = useState<OutreachDraftDto | null>(null);
  const [editedText, setEditedText] = useState('');
  const [tracesOpen, setTracesOpen] = useState(false);

  useEffect(() => {
    if (!tone && tones && tones.length > 0) {
      setTone(tones.find((t) => t.default)?.value ?? tones[0].value);
    }
  }, [tones, tone]);

  const hasContacts = !!contacts && contacts.length > 0;
  const contactChoiceRequired = (contacts?.length ?? 0) > 1 && !contactId;

  const handleGenerate = () => {
    if (contactChoiceRequired) {
      emitToast({ title: 'Choose a contact first', description: 'This job has more than one resolved contact.', variant: 'error' });
      return;
    }
    generate.mutate(
      { contactId: contactId || undefined, tone: tone || undefined },
      {
        onSuccess: (data) => {
          setDraft(data);
          setEditedText(data.text);
          setTracesOpen(false);
        },
        onError: (err) => emitToast({ title: 'Outreach draft failed', description: toErrorMessage(err), variant: 'error' }),
      },
    );
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(editedText);
      emitToast({ title: 'Draft copied', description: 'Paste it into your own email or LinkedIn to send it.', variant: 'success' });
    } catch (err) {
      emitToast({ title: 'Copy failed', description: toErrorMessage(err), variant: 'error' });
    }
  };

  return (
    <>
      <p className="mb-3 text-xs text-faint">
        Draft-only: this generates a message for you to copy and send yourself. Nothing here is ever sent
        automatically.
      </p>

      <div className="mb-3 grid gap-3 sm:grid-cols-2">
        <Field label="Contact">
          <Select
            value={contactId}
            onChange={(e) => setContactId(e.target.value)}
            disabled={!hasContacts}
            data-testid="outreach-contact-select"
          >
            <option value="">{hasContacts ? 'Choose a contact…' : 'No resolved contact — neutral greeting'}</option>
            {(contacts ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.title ? `${c.name} — ${c.title}` : c.name}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="Tone">
          <Select
            value={tone}
            onChange={(e) => setTone(e.target.value as OutreachTone)}
            data-testid="outreach-tone-select"
          >
            {(tones ?? []).map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <div className="mb-3">
        <Button onClick={handleGenerate} disabled={generate.isPending}>
          {generate.isPending ? <Spinner /> : <Sparkles className="h-4 w-4" aria-hidden="true" />}
          {draft ? 'Regenerate' : 'Generate draft'}
        </Button>
      </div>

      {generate.isPending ? <Spinner label="drafting outreach message… (local LLM, be patient)" /> : null}
      {generate.isError ? <ErrorState error={generate.error} /> : null}

      {draft ? <DraftBody draft={draft} editedText={editedText} onEditedTextChange={setEditedText} onCopy={handleCopy} tracesOpen={tracesOpen} onToggleTraces={() => setTracesOpen((v) => !v)} /> : null}
    </>
  );
}

function DraftBody({
  draft,
  editedText,
  onEditedTextChange,
  onCopy,
  tracesOpen,
  onToggleTraces,
}: {
  draft: OutreachDraftDto;
  editedText: string;
  onEditedTextChange: (text: string) => void;
  onCopy: () => void;
  tracesOpen: boolean;
  onToggleTraces: () => void;
}) {
  return (
    <div className="space-y-2" data-testid="outreach-draft">
      <div className="flex flex-wrap items-center gap-2 text-xs text-faint">
        <Chip>{draft.tone}</Chip>
        {draft.contactName ? <span>addressed to: {draft.contactName}</span> : <span>no named contact — neutral greeting</span>}
      </div>

      <Textarea
        className="h-32"
        value={editedText}
        onChange={(e) => onEditedTextChange(e.target.value)}
        data-testid="outreach-draft-text"
      />

      <div className="flex flex-wrap items-center gap-2">
        <Button variant="secondary" onClick={onCopy}>
          copy
        </Button>
        {draft.groundingTraces.length > 0 ? (
          <button
            type="button"
            className="inline-flex items-center gap-1 text-xs text-muted hover:text-foreground"
            onClick={onToggleTraces}
            data-testid="outreach-traces-toggle"
          >
            {draft.groundingTraces.length} grounded claim{draft.groundingTraces.length === 1 ? '' : 's'}
            {tracesOpen ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          </button>
        ) : (
          <span className="text-xs italic text-faint">
            No specific claims — no company-intel signals were available to ground one.
          </span>
        )}
      </div>

      {tracesOpen && draft.groundingTraces.length > 0 ? (
        <ul className="space-y-1.5 rounded-md border border-border bg-surface-secondary/60 p-3" data-testid="outreach-traces">
          {draft.groundingTraces.map((tr, i) => (
            <li key={`${tr.signalKind}-${i}`} className="text-xs">
              <span className="font-medium text-foreground">&ldquo;{tr.claim}&rdquo;</span>
              <span className="text-faint"> — from {tr.signalKind}: {tr.signalValue}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
