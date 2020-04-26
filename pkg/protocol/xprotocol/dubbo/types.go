/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dubbo

const (
	ProtocolName = "dubbo"
)

// dubbo protocol
const (
	HeaderLen   = 16
	IdLen       = 8
	MagicIdx    = 0
	FlagIdx     = 2
	StatusIdx   = 3
	IdIdx       = 4
	DataLenIdx  = 12
	DataLenSize = 4
)

// req/resp type
const (
	CmdTypeResponse      byte   = 0 // cmd type
	CmdTypeRequest       byte   = 1
	CmdTypeRequestOneway byte   = 2
	UnKnownCmdType       string = "unknown cmd type"
)

const (
	EventRequest  int = 1
	EventResponse int = 0
)

const (
	ServiceNameHeader string = "service"
	MethodNameHeader  string = "method"
)

const (
	ResponseStatusSuccess uint16 = 0x14 // 0x14 response status
)
