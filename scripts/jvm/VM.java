import java.nio.file.*;

// Mirrors the injected smali Lshield/rt/VM; run()/i4()/i2() line-by-line, so
// running it on Go-produced bytecode validates that the on-device interpreter
// algorithm is correct (JVM int semantics match Android ART): arithmetic,
// branches (#3/#14 slice 1) and the extended integer ALU (#14 slice 2). Used by
// scripts/validate-vm.sh. Input: line1=wire(hex) line2=bytecode(hex) then
// "a b expected" lines.
public class VM {
  static byte[] hex(String s){ byte[] b=new byte[s.length()/2];
    for(int i=0;i<b.length;i++) b[i]=(byte)Integer.parseInt(s.substring(2*i,2*i+2),16); return b; }
  static int i4(byte[] bc,int pc){
    return (bc[pc]<<24)|((bc[pc+1]&0xff)<<16)|((bc[pc+2]&0xff)<<8)|(bc[pc+3]&0xff); }
  static int i2(byte[] bc,int pc){ return ((bc[pc]&0xff)<<8)|(bc[pc+1]&0xff); }

  static int run(byte[] bc,int[] args,byte[] w){
    int[] o=new int[w.length];
    for(int i=0;i<w.length;i++) o[i]=w[i]&0xff;
    int LOADARG=o[0],CONST=o[1],MOVE=o[2],ADD=o[3],SUB=o[4],MUL=o[5],AND=o[6],OR=o[7],XOR=o[8],
        ADDLIT=o[9],MULLIT=o[10],RET=o[11],GOTO=o[12],
        IFEQ=o[13],IFNE=o[14],IFLT=o[15],IFGE=o[16],IFGT=o[17],IFLE=o[18],
        IFEQZ=o[19],IFNEZ=o[20],IFLTZ=o[21],IFGEZ=o[22],IFGTZ=o[23],IFLEZ=o[24],
        DIV=o[25],REM=o[26],SHL=o[27],SHR=o[28],USHR=o[29],NEG=o[30],NOT=o[31],
        ANDLIT=o[32],ORLIT=o[33],XORLIT=o[34],SHLLIT=o[35],SHRLIT=o[36],USHRLIT=o[37],
        DIVLIT=o[38],REMLIT=o[39],RSUBLIT=o[40],I2B=o[41],I2S=o[42],I2C=o[43];
    int numRegs=bc[0]&0xff;
    int[] r=new int[numRegs];
    int pc=1;
    while(true){
      int op=bc[pc]&0xff; pc++;
      if(op==LOADARG){int d=bc[pc++]&0xff,ai=bc[pc++]&0xff; r[d]=args[ai]; continue;}
      if(op==CONST){int d=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=im; continue;}
      if(op==MOVE){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; r[d]=r[s]; continue;}
      if(op==ADD){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]+r[b]; continue;}
      if(op==SUB){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]-r[b]; continue;}
      if(op==MUL){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]*r[b]; continue;}
      if(op==AND){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]&r[b]; continue;}
      if(op==OR){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]|r[b]; continue;}
      if(op==XOR){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]^r[b]; continue;}
      if(op==DIV){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]/r[b]; continue;}
      if(op==REM){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]%r[b]; continue;}
      if(op==SHL){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]<<r[b]; continue;}
      if(op==SHR){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]>>r[b]; continue;}
      if(op==USHR){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]>>>r[b]; continue;}
      if(op==NEG){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; r[d]=-r[s]; continue;}
      if(op==NOT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; r[d]=~r[s]; continue;}
      if(op==ADDLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]+im; continue;}
      if(op==MULLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]*im; continue;}
      if(op==ANDLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]&im; continue;}
      if(op==ORLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]|im; continue;}
      if(op==XORLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]^im; continue;}
      if(op==SHLLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]<<im; continue;}
      if(op==SHRLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]>>im; continue;}
      if(op==USHRLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]>>>im; continue;}
      if(op==DIVLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]/im; continue;}
      if(op==REMLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]%im; continue;}
      if(op==RSUBLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=im-r[s]; continue;}
      if(op==I2B){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; r[d]=(byte)r[s]; continue;}
      if(op==I2S){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; r[d]=(short)r[s]; continue;}
      if(op==I2C){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; r[d]=(char)r[s]; continue;}
      if(op==RET){int s=bc[pc++]&0xff; return r[s];}
      if(op==GOTO){pc=i2(bc,pc); continue;}
      if(op==IFEQ){int a=bc[pc++]&0xff,b=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]==r[b]) pc=t; continue;}
      if(op==IFNE){int a=bc[pc++]&0xff,b=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]!=r[b]) pc=t; continue;}
      if(op==IFLT){int a=bc[pc++]&0xff,b=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]<r[b]) pc=t; continue;}
      if(op==IFGE){int a=bc[pc++]&0xff,b=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]>=r[b]) pc=t; continue;}
      if(op==IFGT){int a=bc[pc++]&0xff,b=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]>r[b]) pc=t; continue;}
      if(op==IFLE){int a=bc[pc++]&0xff,b=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]<=r[b]) pc=t; continue;}
      if(op==IFEQZ){int a=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]==0) pc=t; continue;}
      if(op==IFNEZ){int a=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]!=0) pc=t; continue;}
      if(op==IFLTZ){int a=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]<0) pc=t; continue;}
      if(op==IFGEZ){int a=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]>=0) pc=t; continue;}
      if(op==IFGTZ){int a=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]>0) pc=t; continue;}
      if(op==IFLEZ){int a=bc[pc++]&0xff,t=i2(bc,pc); pc+=2; if(r[a]<=0) pc=t; continue;}
      return -1;
    }
  }

  public static void main(String[] a) throws Exception {
    var lines=Files.readAllLines(Paths.get(a[0]));
    byte[] w=hex(lines.get(0)), bc=hex(lines.get(1));
    boolean allok=true;
    for(int i=2;i<lines.size();i++){
      var p=lines.get(i).trim().split("\\s+");
      int x=Integer.parseInt(p[0]),y=Integer.parseInt(p[1]),exp=Integer.parseInt(p[2]);
      int got=run(bc,new int[]{x,y},w);
      boolean ok=got==exp; allok&=ok;
      System.out.println("run("+x+","+y+")="+got+" expected="+exp+(ok?" OK":" MISMATCH"));
    }
    System.out.println(allok?"VM-JVM-VALIDATION OK":"VM-JVM-VALIDATION FAILED");
    if(!allok) System.exit(1);
  }
}
