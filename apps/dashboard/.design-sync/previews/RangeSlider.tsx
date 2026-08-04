import { Field, RangeSlider } from '@job-finder/dashboard';

const noop = () => {};

export const ScoreRange = () => (
  <div className="max-w-sm">
    <RangeSlider
      min={0}
      max={100}
      valueMin={55}
      valueMax={90}
      onChangeMin={noop}
      onChangeMax={noop}
      labelMin="minimum score"
      labelMax="maximum score"
    />
  </div>
);

export const FullSpan = () => (
  <div className="max-w-sm">
    <RangeSlider
      min={0}
      max={100}
      valueMin={0}
      valueMax={100}
      onChangeMin={noop}
      onChangeMax={noop}
      labelMin="minimum score"
      labelMax="maximum score"
    />
  </div>
);

export const Labelled = () => (
  <div className="max-w-sm">
    <Field label="Match score">
      <RangeSlider
        min={0}
        max={100}
        step={5}
        valueMin={60}
        valueMax={85}
        onChangeMin={noop}
        onChangeMax={noop}
        labelMin="minimum match score"
        labelMax="maximum match score"
      />
    </Field>
  </div>
);
