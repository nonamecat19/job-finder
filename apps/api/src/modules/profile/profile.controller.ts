import {
  BadRequestException,
  Body,
  Controller,
  Delete,
  Get,
  Param,
  Post,
  Put,
  UploadedFile,
  UseInterceptors,
} from '@nestjs/common';
import { FileInterceptor } from '@nestjs/platform-express';
import { JsonResume } from '@job-finder/shared';
import { ProfileService } from './profile.service';

@Controller('profiles')
export class ProfileController {
  constructor(private readonly profiles: ProfileService) {}

  @Get()
  list() {
    return this.profiles.list();
  }

  @Get(':id')
  get(@Param('id') id: string) {
    return this.profiles.get(id);
  }

  @Post()
  create(@Body() body: { name: string; document: JsonResume; extraNotes?: string }) {
    return this.profiles.create(body);
  }

  @Put(':id')
  update(
    @Param('id') id: string,
    @Body() body: { name?: string; document?: JsonResume; extraNotes?: string | null },
  ) {
    return this.profiles.update(id, body);
  }

  @Delete(':id')
  remove(@Param('id') id: string) {
    return this.profiles.remove(id);
  }

  @Post('import')
  @UseInterceptors(FileInterceptor('file'))
  import(@UploadedFile() file: Express.Multer.File) {
    if (!file?.buffer) throw new BadRequestException('multipart field "file" (PDF) is required');
    return this.profiles.importPdf(file.buffer);
  }
}
