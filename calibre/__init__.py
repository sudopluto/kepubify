#!/usr/bin/env python2
from __future__ import with_statement

import os, shutil, re, subprocess, platform, stat, io

from calibre.customize.conversion import (OutputFormatPlugin, OptionRecommendation)
from calibre.ptempfile import TemporaryDirectory
from calibre import CurrentDir
from calibre.constants import filesystem_encoding

class KepubifyOutput(OutputFormatPlugin):
    name = 'Kepubify Output'
    author = 'Patrick Gaskin'
    description = 'Converts epubs to kepubs using kepubify. Conflicts with KePub Output Plugin.'
    file_type = 'kepub'
    commit_name = 'kepubify_output'
    version = (0, 0, 1)
    supported_platforms = ['windows', 'osx', 'linux']
    kepubify_path = None

    def initialize(self):
        tkdir = TemporaryDirectory(u'kepubify')
        kdir = tkdir.dir

        if platform.system() == 'Linux':
            kfn = None
            if platform.machine() == 'x86' or platform.machine() == 'i386' or platform.machine() == 'i686':
                kfn = 'kepubify-linux-32bit'
            elif platform.machine() == 'x86_64' or platform.machine() == 'x64':
                kfn = 'kepubify-linux-64bit'
            else:
                raise Exception('unsupported architecture (currently only 64 and 32bit)')

            kepubify = self.load_resources([kfn])[kfn]
            self.kepubify_path = os.path.join(kdir, kfn)
            with io.open(self.kepubify_path, 'wb') as f:
                f.write(kepubify)
            st = os.stat(self.kepubify_path)
            os.chmod(self.kepubify_path, st.st_mode | stat.S_IEXEC)
            subprocess.check_call([self.kepubify_path, "--version"])
        elif platform.system() == 'Windows':
            kfn = 'kepubify-windows-32bit.exe'
            kepubify = self.load_resources([kfn])[kfn]
            self.kepubify_path = os.path.join(kdir, kfn)
            with io.open(self.kepubify_path, 'wb') as f:
                f.write(kepubify)
            subprocess.check_call([self.kepubify_path, "--version"])
        elif platform.system() == 'Darwin':
            kfn = 'kepubify-darwin-64bit'
            kepubify = self.load_resources([kfn])[kfn]
            self.kepubify_path = os.path.join(kdir, kfn)
            with io.open(self.kepubify_path, 'wb') as f:
                f.write(kepubify)
            st = os.stat(self.kepubify_path)
            os.chmod(self.kepubify_path, st.st_mode | stat.S_IEXEC)
            subprocess.check_call([self.kepubify_path, "--version"])
        else:
            raise Exception('unsupported platform')

    def convert(self, oeb, output_path, input_plugin, opts, log):
        self.log, self.opts, self.oeb = log, opts, oeb

        from calibre.ebooks.oeb.transforms.cover import CoverManager
        cm = CoverManager(no_default_cover=False, no_svg_cover=True, preserve_aspect_ratio=True)
        cm(self.oeb, self.opts, self.log)

        if self.oeb.toc.count() == 0:
            self.log.warn('This EPUB file has no Table of Contents. Creating a default TOC')
            first = iter(self.oeb.spine).next()
            self.oeb.toc.add(_('Start'), first.href)

        from calibre.ebooks.oeb.base import OPF
        uuid = None
        for x in oeb.metadata['identifier']:
            if x.get(OPF('scheme'), None).lower() == 'uuid' or unicode(x).startswith('urn:uuid:'):
                uuid = unicode(x).split(':')[-1]
                break
        
        if uuid is None:
            self.log.warn('No UUID identifier found')
            from uuid import uuid4
            uuid = str(uuid4())
            oeb.metadata.add('identifier', uuid, scheme='uuid', id=uuid)

        with TemporaryDirectory(u'_kepubify_output') as tdir:
            from calibre.customize.ui import plugin_for_output_format
            extra_entries = []
            oeb_output = plugin_for_output_format('oeb')
            oeb_output.convert(oeb, tdir, input_plugin, opts, log)
            opf = [x for x in os.listdir(tdir) if x.endswith('.opf')][0]
            self.condense_ncx([os.path.join(tdir, x) for x in os.listdir(tdir) if x.endswith('.ncx')][0])

            from calibre.ebooks.epub import initialize_container
            with initialize_container(output_path.replace('kepub', 'epub'), os.path.basename(opf), extra_entries=extra_entries) as epub:
                epub.add_dir(tdir)
            
            self.log.debug(subprocess.check_output([self.kepubify_path, '-o', tdir, output_path.replace('kepub', 'epub')]))
            self.log.debug(os.listdir(tdir))

            os.remove(output_path.replace('kepub', 'epub'))
            os.rename(os.path.join(tdir, [x for x in os.listdir(tdir) if x.endswith('kepub.epub')][0]), output_path)
    
    def condense_ncx(self, ncx_path):
        from lxml import etree
        if not self.opts.pretty_print:
            tree = etree.parse(ncx_path)
            for tag in tree.getroot().iter(tag=etree.Element):
                if tag.text:
                    tag.text = tag.text.strip()
                if tag.tail:
                    tag.tail = tag.tail.strip()
            compressed = etree.tostring(tree.getroot(), encoding='utf-8')
            open(ncx_path, 'wb').write(compressed)