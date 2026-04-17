import { z } from 'zod';

/** Zod mirror of the JSON Resume subset in @job-finder/shared — used for LLM structured parsing. */
export const jsonResumeSchema = z.object({
  basics: z
    .object({
      name: z.string().optional(),
      label: z.string().optional(),
      email: z.string().optional(),
      phone: z.string().optional(),
      url: z.string().optional(),
      summary: z.string().optional(),
      location: z
        .object({
          city: z.string().optional(),
          countryCode: z.string().optional(),
          region: z.string().optional(),
        })
        .optional(),
      profiles: z
        .array(
          z.object({
            network: z.string().optional(),
            username: z.string().optional(),
            url: z.string().optional(),
          }),
        )
        .optional(),
    })
    .optional(),
  work: z
    .array(
      z.object({
        name: z.string(),
        position: z.string().optional(),
        url: z.string().optional(),
        startDate: z.string().optional(),
        endDate: z.string().optional(),
        summary: z.string().optional(),
        highlights: z.array(z.string()).optional(),
      }),
    )
    .optional(),
  education: z
    .array(
      z.object({
        institution: z.string(),
        area: z.string().optional(),
        studyType: z.string().optional(),
        startDate: z.string().optional(),
        endDate: z.string().optional(),
      }),
    )
    .optional(),
  skills: z
    .array(
      z.object({
        name: z.string(),
        level: z.string().optional(),
        keywords: z.array(z.string()).optional(),
      }),
    )
    .optional(),
  projects: z
    .array(
      z.object({
        name: z.string(),
        description: z.string().optional(),
        url: z.string().optional(),
        keywords: z.array(z.string()).optional(),
        highlights: z.array(z.string()).optional(),
      }),
    )
    .optional(),
  languages: z
    .array(z.object({ language: z.string().optional(), fluency: z.string().optional() }))
    .optional(),
  certificates: z
    .array(
      z.object({
        name: z.string().optional(),
        issuer: z.string().optional(),
        date: z.string().optional(),
      }),
    )
    .optional(),
});

export type ParsedResume = z.infer<typeof jsonResumeSchema>;
