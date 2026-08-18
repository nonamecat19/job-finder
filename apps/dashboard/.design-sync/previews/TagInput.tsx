import { Field, TagInput } from '@job-finder/dashboard';

const noop = () => {};

export const WithTags = () => (
  <div className="max-w-sm">
    <TagInput values={['Go', 'PostgreSQL', 'Kafka']} onChange={noop} aria-label="Skill details" />
  </div>
);

export const Empty = () => (
  <div className="max-w-sm">
    <TagInput values={[]} onChange={noop} placeholder="Python, Go, Rust" aria-label="Skill details" />
  </div>
);

export const InField = () => (
  <div className="max-w-sm">
    <Field label="Details">
      <TagInput
        values={['Python', 'Rust', 'TypeScript', 'gRPC']}
        onChange={noop}
        placeholder="Python, Go, Rust"
        aria-label="Skill details"
      />
    </Field>
  </div>
);
