import { Injectable, Logger } from '@nestjs/common';
import { execFile } from 'child_process';
import { mkdir, writeFile } from 'fs/promises';
import * as path from 'path';
import { promisify } from 'util';
import { stringify as yamlStringify } from 'yaml';
import { RendercvMaster } from './rendercv-tailor';

const execFileAsync = promisify(execFile);

/**
 * Renders a (tailored) RenderCV master object to PDF by writing it back to YAML and
 * shelling out to the `rendercv` CLI. Sits alongside {@link HtmlPdfRenderer}: the
 * JSON-Resume path uses Handlebars + Puppeteer, this path uses the user's real
 * rendercv config so the Typst theme/design is preserved exactly.
 */
@Injectable()
export class RenderCvRenderer {
  private readonly logger = new Logger(RenderCvRenderer.name);

  private get outDir(): string {
    return process.env.DOCUMENTS_DIR ?? '/data/documents';
  }

  private get bin(): string {
    return process.env.RENDERCV_BIN ?? 'rendercv';
  }

  /**
   * @param master merged rendercv config (design/templates/locale intact)
   * @param baseName sanitized file stem, e.g. "acme-backend-engineer-v1"
   * @returns absolute path to the produced PDF
   */
  async render(master: RendercvMaster, baseName: string): Promise<{ yamlPath: string; pdfPath: string }> {
    await mkdir(this.outDir, { recursive: true });
    const yamlPath = path.join(this.outDir, `${baseName}.yaml`);
    const pdfPath = path.join(this.outDir, `${baseName}.pdf`);

    await writeFile(yamlPath, yamlStringify(master), 'utf8');

    // -pdf path is relative to the input file, so pass just the basename.
    const { stderr } = await execFileAsync(
      this.bin,
      ['render', yamlPath, '-o', this.outDir, '-pdf', `${baseName}.pdf`, '-nopng', '-nohtml', '-nomd'],
      { cwd: this.outDir, timeout: 120_000 },
    );
    if (stderr?.trim()) this.logger.debug(`rendercv stderr: ${stderr.trim()}`);

    return { yamlPath, pdfPath };
  }
}
