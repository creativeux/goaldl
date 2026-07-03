//--------------------------------------------------------------------------------------
// Created by TunerPro. Hand editing is *not* recommended or supported.
//--------------------------------------------------------------------------------------


//--------------------------------------------------------------------------------------
//--------------------------------- HEADER ------------------------------------
//--------------------------------------------------------------------------------------

{
	fDefFrmtVers             =1.21;
	strDefVersion            =Version 1.0;
	strDefTitle              =A58;
	strAuthor                =Robert Saar;
	strEngine                =4.3, 5.0, 5.7, 6.0, 7.4 TBI;
	strYear                  =1989-1991;
	strVINCode               =Z, B, H, K, P,;
	strCodeMask              =$42;
	strComments              =Put The 10K resistor over A-B;
	iBaud                    =160;
	dwFlags                  =0x00000000;
	dwCSID                   =0x00011433;
	btNumDumpRequests        =1;

	strCommandName           =Data Transfer;
	rgbtCommand              =;
	iTotalBytesInCommand     =0;
	bChecksumCommand         =0;
	iNumBytesInPayload       =20;
	iNumBytesBeforePayload   =-1;
	bMaster                  =1;
	bMonitor                 =1;
	iChainTo                 =-1;
}

//--------------------------------------------------------------------------------------
//---------------------------------- DASH -------------------------------------
//--------------------------------------------------------------------------------------

{
	dwItemType               =6;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =0;

	btNumGauges              =6;
	strIDsDisplayed          =0,0,0,0,0,0,;
	btNumMonitors            =4;
	strMonsDisplayed         =18,60,0,0,;
}

//--------------------------------------------------------------------------------------
//--------------------------------- VALUES ------------------------------------
//--------------------------------------------------------------------------------------

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =1;
	bVisible                 =1;
	dwUniqueID               =61;

	btByteNumber             =0;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             ======Important=====;
	strUnitLabel             =Units;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =12;

	btByteNumber             =6;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =3;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =Vehicle Speed;
	strUnitLabel             =MPH;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =14;

	btByteNumber             =8;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =25.000000;
	dOffset                  =0.000000;
	strItemTitle             =Engine Speed;
	strUnitLabel             =RPM;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =1;
	bVisible                 =1;
	dwUniqueID               =62;

	btByteNumber             =0;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             ======Air/Fuel=====;
	strUnitLabel             =Units;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =59;

	btByteNumber             =19;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =3;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =Block Learn Multiplier (BLM);
	strUnitLabel             =;
	dwAlarmHigh              =145;
	bAlarmHighENable         =1;
	dwAlarmLow               =108;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =17;

	btByteNumber             =10;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =3;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =Integrator (INT);
	strUnitLabel             =;
	dwAlarmHigh              =145;
	bAlarmHighENable         =1;
	dwAlarmLow               =108;
	bAlarmLowEnable          =1;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =18;

	btByteNumber             =11;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =4.340000;
	dOffset                  =0.000000;
	strItemTitle             =O2 Sensor;
	strUnitLabel             =mV;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =60;

	btByteNumber             =20;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =3;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =O2 Crosses;
	strUnitLabel             =Crosses;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =1;
	bVisible                 =1;
	dwUniqueID               =63;

	btByteNumber             =0;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             ======MISC=====;
	strUnitLabel             =Units;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =16;

	btByteNumber             =18;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =3;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =Knock Counter;
	strUnitLabel             =Knocks;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =15;

	btByteNumber             =9;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =0.019600;
	dOffset                  =0.000000;
	strItemTitle             =Throttle Position Sensor (TPS);
	strUnitLabel             =Volts;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =67;

	btByteNumber             =9;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =4;
	dFactor                  =0.019600;
	dOffset                  =0.000000;
	strItemTitle             =TPS %;
	strUnitLabel             =%;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =225;
	iRangeLow                =30;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =13;

	btByteNumber             =7;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =0.019600;
	dOffset                  =0.000000;
	strItemTitle             =Manifold Absolute Pressure (MAP);
	strUnitLabel             =Volts;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =66;

	btByteNumber             =7;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =0.369000;
	dOffset                  =10.354000;
	strItemTitle             =MAP kPa;
	strUnitLabel             =kPa;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =9;

	btByteNumber             =5;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =6;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =Coolant Temp;
	strUnitLabel             =*F;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =11;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =51;

	btByteNumber             =16;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =0.100000;
	dOffset                  =0.000000;
	strItemTitle             =Battery Voltage;
	strUnitLabel             =Volts;
	dwAlarmHigh              =160;
	bAlarmHighENable         =1;
	dwAlarmLow               =120;
	bAlarmLowEnable          =1;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =1;

	btByteNumber             =4;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =3;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =IAC Position;
	strUnitLabel             =Steps;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

{
	dwItemType               =1;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =7;

	btByteNumber             =15;
	btMessageNumber          =1;
	dwItemSizeBits           =8;
	dwOperation              =0;
	dFactor                  =1.000000;
	dOffset                  =0.000000;
	strItemTitle             =TEMP - Loop Status;
	strUnitLabel             =Units;
	dwAlarmHigh              =255;
	bAlarmHighENable         =0;
	dwAlarmLow               =0;
	bAlarmLowEnable          =0;
	iRangeHigh               =255;
	iRangeLow                =0;
	iLookupTableIndex        =-1;
}

//--------------------------------------------------------------------------------------
//---------------------------------- BITS -------------------------------------
//--------------------------------------------------------------------------------------

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =64;

	btByteNumber             =0;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             ======MALFUNCTION CODES=====;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =1;
	strBitClearTitle         =0;
}

{
	dwItemType               =2;
	strItemComments          =not necessarily right.;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =37;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =2;
	strItemTitle             =OverVoltage;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =45;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =2;
	strItemTitle             =Low Battery Voltage;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =20;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =1;
	strItemTitle             =MAT/IAT;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =34;

	btByteNumber             =13;
	btMessageNumber          =1;
	btBitNumber              =7;
	strItemTitle             =MAT/IAT;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =22;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =3;
	strItemTitle             =TPS High;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =21;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =2;
	strItemTitle             =TPS Low;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =24;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =5;
	strItemTitle             =Coolant Sensor High;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =23;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =4;
	strItemTitle             =Coolant Sensor Low;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =31;

	btByteNumber             =13;
	btMessageNumber          =1;
	btBitNumber              =4;
	strItemTitle             =MAP High;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =30;

	btByteNumber             =13;
	btMessageNumber          =1;
	btBitNumber              =3;
	strItemTitle             =MAP Low;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =40;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =5;
	strItemTitle             =O2 Sensor Rich;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =41;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =6;
	strItemTitle             =O2 Sensor Lean;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =25;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =6;
	strItemTitle             =Oxygen Sensor;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =32;

	btByteNumber             =13;
	btMessageNumber          =1;
	btBitNumber              =5;
	strItemTitle             =EGR;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =19;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             =VSS;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =Will also be set when engine isn't running;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =26;

	btByteNumber             =12;
	btMessageNumber          =1;
	btBitNumber              =7;
	strItemTitle             =Crank Sensor;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =29;

	btByteNumber             =13;
	btMessageNumber          =1;
	btBitNumber              =2;
	strItemTitle             =IAC;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =36;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =1;
	strItemTitle             =Fuel Pump Relay;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =39;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =4;
	strItemTitle             =PROM;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =42;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =7;
	strItemTitle             =ESC (Knock Sensor);
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =38;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =3;
	strItemTitle             =CAL PACK Missing;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =not entirely sure about this one.;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =28;

	btByteNumber             =13;
	btMessageNumber          =1;
	btBitNumber              =1;
	strItemTitle             =Cylinder Select;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =no idea what a ADU is, but its ECM related;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =35;

	btByteNumber             =14;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             =ADU;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =27;

	btByteNumber             =13;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             =EST;
	bAlarmSetEnable          =1;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ERROR;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =1;
	bVisible                 =1;
	dwUniqueID               =65;

	btByteNumber             =0;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             ======OTHER=====;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =1;
	strBitClearTitle         =0;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =4;

	btByteNumber             =1;
	btMessageNumber          =1;
	btBitNumber              =3;
	strItemTitle             =Factory Test Mode;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =Enabled;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =5;

	btByteNumber             =1;
	btMessageNumber          =1;
	btBitNumber              =4;
	strItemTitle             =Diagnostic Mode;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =Enabled;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =must use the 10K resistor to function;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =6;

	btByteNumber             =1;
	btMessageNumber          =1;
	btBitNumber              =5;
	strItemTitle             =ALDL Mode;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =Enabled;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =not sure if this is meant for high or low temp;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =8;

	btByteNumber             =1;
	btMessageNumber          =1;
	btBitNumber              =7;
	strItemTitle             =Idle Mode;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =YES;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =58;

	btByteNumber             =17;
	btMessageNumber          =1;
	btBitNumber              =7;
	strItemTitle             =A/C Status;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =;
	strBitClearTitle         =REQUESTED;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =54;

	btByteNumber             =17;
	btMessageNumber          =1;
	btBitNumber              =2;
	strItemTitle             =A/C Disable;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =DISABLED;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =has something to do with the injector(s) and how they fire;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =47;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =4;
	strItemTitle             =Async Flag;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ASYNC;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =56;

	btByteNumber             =17;
	btMessageNumber          =1;
	btBitNumber              =4;
	strItemTitle             =Shifter Position;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =PARK/NEUTRAL;
	strBitClearTitle         =DRIVE;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =57;

	btByteNumber             =17;
	btMessageNumber          =1;
	btBitNumber              =5;
	strItemTitle             =High Gear;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =;
	strBitClearTitle         =Switched Off;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =55;

	btByteNumber             =17;
	btMessageNumber          =1;
	btBitNumber              =3;
	strItemTitle             =TCC;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =LOCKED;
	strBitClearTitle         =UNLOCKED;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =52;

	btByteNumber             =17;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             =AIR Switch;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ENGAGED;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =53;

	btByteNumber             =17;
	btMessageNumber          =1;
	btBitNumber              =1;
	strItemTitle             =AIR Divert;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ENGAGED;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =no idea what its meant for;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =48;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =5;
	strItemTitle             =Old High Gear;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =;
	strBitClearTitle         =High Gear Last;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =46;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =3;
	strItemTitle             =4-3 Downshift for TCC Unlock;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =YES;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =2;

	btByteNumber             =1;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             =VSS Pulse;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =Yes;
	strBitClearTitle         =No;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =3;

	btByteNumber             =1;
	btMessageNumber          =1;
	btBitNumber              =2;
	strItemTitle             =Crank Sensor Pulse;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =Yes;
	strBitClearTitle         =No;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =44;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =1;
	strItemTitle             =BLM;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =ENABLE;
	strBitClearTitle         =;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =49;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =6;
	strItemTitle             =Rich/Lean;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =Rich;
	strBitClearTitle         =Lean;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =50;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =7;
	strItemTitle             =Loop Status;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =1;
	strBitSetTitle           =CLOSED;
	strBitClearTitle         =OPEN;
}

{
	dwItemType               =2;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =43;

	btByteNumber             =15;
	btMessageNumber          =1;
	btBitNumber              =0;
	strItemTitle             =Idle Closed Loop;
	bAlarmSetEnable          =0;
	bAlarmNotSetEnable       =0;
	strBitSetTitle           =CLOSED;
	strBitClearTitle         =OPEN;
}

//--------------------------------------------------------------------------------------
//---------------------------- LOOKUP TABLES ----------------------------------
//--------------------------------------------------------------------------------------

{
	dwItemType               =5;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =10;

	btDataType               =2;
	wTableSize               =256;
	wIndexSize               =4;
	strTableName             =Coolant Temp in C;
	dwReserved               =0;
	dwReserved               =0;
	pbtData                  =0, 200.00
				 12, 150.00
				 13, 145.00
				 14, 140.00
				 16, 135.00
				 18, 130.00
				 21, 125.00
				 23, 120.00
				 26, 115.00
				 30, 110.00
				 34, 105.00
				 39, 100.00
				 44, 95.00
				 50, 90.00
				 56, 85.00
				 64, 80.00
				 72, 75.00
				 81, 70.00
				 92, 65.00
				 102, 60.00
				 114, 55.00
				 126, 50.00
				 139, 45.00
				 152, 40.00
				 165, 35.00
				 177, 30.00
				 189, 25.00
				 199, 20.00
				 209, 15.00
				 218, 10.00
				 225, 5.00
				 231, 0.00
				 237, -5.00
				 241, -10.00
				 245, -15.00
				 247, -20.00
				 250, -25.00
				 251, -30.00
				 255, -40.00;
}

{
	dwItemType               =5;
	strItemComments          =<Comments>;
	bSeparator               =0;
	bVisible                 =1;
	dwUniqueID               =11;

	btDataType               =2;
	wTableSize               =256;
	wIndexSize               =4;
	strTableName             =Coolant Temp in F;
	dwReserved               =0;
	dwReserved               =0;
	pbtData                  =0, 392.00
				 12, 302.00
				 13, 293.00
				 14, 284.00
				 16, 275.00
				 18, 266.00
				 21, 257.00
				 23, 248.00
				 26, 239.00
				 30, 230.00
				 34, 221.00
				 39, 212.00
				 44, 203.00
				 50, 194.00
				 56, 185.00
				 64, 176.00
				 72, 167.00
				 81, 158.00
				 92, 149.00
				 102, 140.00
				 114, 131.00
				 126, 122.00
				 139, 113.00
				 152, 104.00
				 165, 95.00
				 177, 86.00
				 189, 77.00
				 199, 68.00
				 209, 59.00
				 218, 50.00
				 225, 41.00
				 231, 32.00
				 237, 23.00
				 241, 14.00
				 245, 5.00
				 247, -4.00
				 250, -13.00
				 251, -22.00
				 255, -40.00;
}

