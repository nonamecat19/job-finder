import { Body, Controller, Get, NotFoundException, Param, Post, Put, Res } from '@nestjs/common';
import { Response } from 'express';
import { createReadStream, existsSync } from 'fs';
import * as path from 'path';
import { GenerationService } from './generation.service';
import { GroundingLevel } from './rendercv-tailor';

@Controller('documents')
export class DocumentsController {
  constructor(private readonly generation: GenerationService) {}

  /**
   * Ad-hoc rendercv tailoring from a pasted vacancy (no DB job). Returns the produced
   * YAML + PDF paths. This is the endpoint the opencode `/tailor-resume` command calls.
   */
  @Post('tailor')
  tailor(
    @Body()
    body: { vacancy: string; company?: string; title?: string; groundingLevel?: GroundingLevel },
  ) {
    if (!body?.vacancy?.trim()) throw new NotFoundException('vacancy text is required');
    return this.generation.generateRendercvFromText(body);
  }

  @Get(':id')
  get(@Param('id') id: string) {
    return this.generation.getDocument(id);
  }

  @Put(':id')
  update(@Param('id') id: string, @Body() body: { text?: string }) {
    return this.generation.updateDocument(id, body);
  }

  @Get(':id/pdf')
  async pdf(@Param('id') id: string, @Res() res: Response) {
    const doc = await this.generation.getDocument(id);
    if (!doc.pdfPath || !existsSync(doc.pdfPath)) {
      throw new NotFoundException('PDF not rendered yet');
    }
    res.setHeader('Content-Type', 'application/pdf');
    res.setHeader(
      'Content-Disposition',
      `attachment; filename="${path.basename(doc.pdfPath)}"`,
    );
    createReadStream(doc.pdfPath).pipe(res);
  }
}
