import sys
import os
import re
from fpdf import FPDF, XPos, YPos

class VyogoPDF(FPDF):
    def __init__(self):
        super().__init__()
        # Load fonts
        self.add_font("Arial", "", "/System/Library/Fonts/Supplemental/Arial.ttf")
        self.add_font("Arial", "B", "/System/Library/Fonts/Supplemental/Arial Bold.ttf")
        self.add_font("CourierNew", "", "/System/Library/Fonts/Supplemental/Courier New.ttf")
        self.set_font("Arial", "", 11) # Increased base font size

    def header(self):
        self.image("logo.png", 10, 8, 25)
        self.set_font("Arial", "B", 14)
        self.set_text_color(6, 85, 163) # Brand Blue #0655A3
        self.cell(0, 10, "Frappe Deployment Runbook", 0, align="R", new_x=XPos.LMARGIN, new_y=YPos.NEXT)
        self.ln(15)

    def footer(self):
        self.set_y(-15)
        self.set_font("Arial", "", 8)
        self.set_text_color(128)
        self.cell(100, 10, "www.vyogo.tech", 0, 0, "L", link="https://vyogo.tech")
        self.cell(0, 10, f"Page {self.page_no()}/{{nb}}", 0, 0, "R")

    def draw_divider(self):
        self.set_draw_color(200, 200, 200) # Gray divider
        self.set_line_width(0.2)
        y = self.get_y() + 2
        self.line(10, y, 200, y)
        self.ln(5)

    def write_rich_text(self, text, size=11, color=(30, 30, 30)):
        # Simple parser for [text](url) and **bold**
        # and `code` markers
        self.set_font("Arial", "", size)
        self.set_text_color(*color)
        
        # This is a basic implementation using write() for inline links
        parts = re.split(r'(\[.*?\]\(.*?\))', text)
        for part in parts:
            link_match = re.match(r'\[(.*?)\]\((.*?)\)', part)
            if link_match:
                label = link_match.group(1)
                url = link_match.group(2)
                self.set_text_color(0, 0, 255) # Standard Blue
                self.set_font("Arial", "U", size)
                self.write(5, label, link=url)
                self.set_text_color(*color)
                self.set_font("Arial", "", size)
            else:
                # Handle **bold** and `code` in regular text
                subparts = re.split(r'(\*\*.*?\*\*|`.*?`)', part)
                for sub in subparts:
                    if sub.startswith('**') and sub.endswith('**'):
                        self.set_font("Arial", "B", size)
                        self.write(5, sub[2:-2].encode('latin-1', 'replace').decode('latin-1'))
                        self.set_font("Arial", "", size)
                    elif sub.startswith('`') and sub.endswith('`'):
                        self.set_font("CourierNew", "", size-1)
                        self.set_fill_color(240, 240, 240)
                        self.write(5, sub[1:-1].encode('latin-1', 'replace').decode('latin-1'))
                        self.set_font("Arial", "", size)
                    else:
                        self.write(5, sub.encode('latin-1', 'replace').decode('latin-1'))
        self.ln(6)

def convert_md_to_pdf(md_path, pdf_path):
    with open(md_path, "r", encoding="utf-8") as f:
        content = f.read()

    pdf = VyogoPDF()
    pdf.set_auto_page_break(auto=True, margin=15)
    pdf.alias_nb_pages()
    pdf.add_page()
    
    lines = content.split('\n')
    in_code_block = False
    
    for line in lines:
        pdf.set_x(10)
        
        if line.startswith('---'):
            pdf.draw_divider()
            continue

        if line.startswith('```'):
            in_code_block = not in_code_block
            pdf.ln(1)
            continue
            
        if in_code_block:
            # DARK MODE: Black bg, White text
            pdf.set_font("CourierNew", "", 8.5)
            pdf.set_text_color(255, 255, 255)
            pdf.set_fill_color(20, 20, 20)
            safe_text = line.encode('latin-1', 'replace').decode('latin-1')
            pdf.multi_cell(0, 4.5, safe_text, fill=True, border=0)
            continue

        if line.startswith('# '):
            pdf.ln(5)
            pdf.set_font("Arial", "B", 20)
            pdf.set_text_color(6, 85, 163)
            pdf.multi_cell(0, 10, line[2:])
            pdf.ln(2)
        elif line.startswith('## '):
            pdf.ln(4)
            pdf.set_font("Arial", "B", 16)
            pdf.set_text_color(6, 85, 163)
            pdf.multi_cell(0, 8, line[3:])
            pdf.ln(2)
        elif line.startswith('### '):
            pdf.ln(3)
            pdf.set_font("Arial", "B", 13)
            pdf.set_text_color(6, 85, 163)
            pdf.multi_cell(0, 7, line[4:])
            pdf.ln(1)
        elif line.startswith('- '):
            pdf.set_x(15)
            pdf.write_rich_text("- " + line[2:], size=11)
        elif line.strip() == "":
            pdf.ln(2)
        else:
            pdf.write_rich_text(line, size=11)

    pdf.output(pdf_path)
    print(f"Branded PDF generated: {pdf_path}")

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Convert Markdown to Branded PDF")
    parser.add_argument("input", nargs="?", default="docs/runbook.md", help="Input Markdown file")
    parser.add_argument("output", nargs="?", default="docs/runbook_branded.pdf", help="Output PDF file")
    
    args = parser.parse_args()
    
    # Ensure directories exist
    os.makedirs(os.path.dirname(args.output), exist_ok=True)
    
    convert_md_to_pdf(args.input, args.output)
